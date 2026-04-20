package scheduler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/worker"
)

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/control/time/set", s.handleSetTime)
	mux.HandleFunc("/control/time/advance", s.handleAdvanceTime)
	mux.HandleFunc("/control/time/clear", s.handleClearTime)
	mux.HandleFunc("/control/jobs/run-due", s.handleRunDue)
	mux.HandleFunc("/control/digests/reconcile", s.handleReconcile)
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
	processed, limitReached, err := worker.DrainDue(r.Context(), s.logger, s.store, "scheduler-control", s, body.Limit)
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
