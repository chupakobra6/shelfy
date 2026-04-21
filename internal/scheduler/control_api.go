package scheduler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/worker"
)

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/control/time/set", s.handleSetTime)
	mux.HandleFunc("/control/time/advance", s.handleAdvanceTime)
	mux.HandleFunc("/control/time/clear", s.handleClearTime)
	mux.HandleFunc("/control/jobs/run-due", s.handleRunDue)
	mux.HandleFunc("/control/digests/reconcile", s.handleReconcile)
	if s.enableE2EReset {
		mux.HandleFunc("/control/e2e/reset", s.handleE2EReset)
	}
	return mux
}

func (s *Service) handleSetTime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := time.Parse(time.RFC3339, strings.TrimSpace(body.Now))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.SetClockOverride(r.Context(), &value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "now": value.UTC().Format(time.RFC3339)})
}

func (s *Service) handleAdvanceTime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Duration string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	delta, err := time.ParseDuration(body.Duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := s.store.AdvanceClock(r.Context(), delta)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "now": value.UTC().Format(time.RFC3339)})
}

func (s *Service) handleClearTime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.store.SetClockOverride(r.Context(), nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Service) handleRunDue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Limit              int   `json:"limit"`
		IncludeMaintenance *bool `json:"include_maintenance"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	includeMaintenance := true
	if body.IncludeMaintenance != nil {
		includeMaintenance = *body.IncludeMaintenance
	}
	if includeMaintenance {
		if err := s.RunMaintenanceTick(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	store, ok := s.store.(*postgres.Store)
	if !ok {
		http.Error(w, "scheduler control requires postgres store", http.StatusInternalServerError)
		return
	}
	processed, limitReached, err := worker.DrainDue(r.Context(), s.logger, store, "scheduler-control", s, body.Limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                  true,
		"processed_jobs":      processed,
		"include_maintenance": includeMaintenance,
		"limit_reached":       limitReached,
	})
}

func (s *Service) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.cleanupDigestMessages(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Service) handleE2EReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.store.ResetE2EUserState(r.Context(), s.e2eTestUserID, s.defaultTimezone, s.digestLocalTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.bestEffortResetCleanup(r.Context(), result)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                            true,
		"user_id":                       result.UserID,
		"chat_id":                       result.ChatID,
		"settings_found":                result.SettingsFound,
		"cleared_dashboard_message_id":  result.ClearedDashboardMessageID,
		"cleanup_attempted_message_ids": result.CleanupAttemptedMessageIDs,
		"deleted": map[string]any{
			"drafts":   result.Deleted.Drafts,
			"products": result.Deleted.Products,
			"digests":  result.Deleted.Digests,
			"jobs":     result.Deleted.Jobs,
		},
	})
}

func (s *Service) bestEffortResetCleanup(ctx context.Context, result postgres.E2EResetResult) {
	if !result.SettingsFound || result.ChatID == 0 {
		return
	}
	for _, messageID := range result.CleanupAttemptedMessageIDs {
		if err := s.tg.DeleteMessage(ctx, result.ChatID, messageID); err != nil {
			if telegram.IsMissingMessageTargetError(err) {
				continue
			}
			s.logger.WarnContext(ctx, "e2e_reset_cleanup_delete_failed", observability.LogAttrs(ctx,
				"user_id", result.UserID,
				"chat_id", result.ChatID,
				"message_id", messageID,
				"error", err,
			)...)
		}
	}
}
