package postgres

import (
	"context"
	"errors"

	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

type E2EResetDeletedCounts struct {
	Drafts   int64 `json:"drafts"`
	Products int64 `json:"products"`
	Digests  int64 `json:"digests"`
	Jobs     int64 `json:"jobs"`
}

type E2EResetResult struct {
	UserID                     int64                 `json:"user_id"`
	ChatID                     int64                 `json:"chat_id"`
	SettingsFound              bool                  `json:"settings_found"`
	ClearedDashboardMessageID  *int64                `json:"cleared_dashboard_message_id,omitempty"`
	CleanupAttemptedMessageIDs []int64               `json:"cleanup_attempted_message_ids"`
	Deleted                    E2EResetDeletedCounts `json:"deleted"`
}

func (s *Store) ResetE2EUserState(ctx context.Context, userID int64, defaultTimezone, digestLocalTime string) (E2EResetResult, error) {
	result := E2EResetResult{
		UserID:                     userID,
		CleanupAttemptedMessageIDs: []int64{},
	}

	settings, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.InfoContext(ctx, "e2e_reset_skipped_missing_settings", observability.LogAttrs(ctx, "user_id", userID)...)
			return result, nil
		}
		return E2EResetResult{}, err
	}

	draftRows, err := s.queries.ListDraftSessionsForUser(ctx, userID)
	if err != nil {
		return E2EResetResult{}, err
	}
	digestRows, err := s.queries.ListActiveDigestMessagesForUser(ctx, userID)
	if err != nil {
		return E2EResetResult{}, err
	}

	result.SettingsFound = true
	result.ChatID = settings.ChatID
	result.ClearedDashboardMessageID = settings.DashboardMessageID

	cleanupIDs := make([]int64, 0, len(draftRows)*4+len(digestRows)+1)
	if settings.DashboardMessageID != nil {
		cleanupIDs = append(cleanupIDs, *settings.DashboardMessageID)
	}
	for _, row := range draftRows {
		cleanupIDs = append(cleanupIDs,
			optionalMessageID(row.SourceMessageID),
			optionalMessageID(row.DraftMessageID),
			optionalMessageID(row.FeedbackMessageID),
			optionalMessageID(row.EditPromptMessageID),
		)
	}
	for _, row := range digestRows {
		cleanupIDs = append(cleanupIDs, row.TelegramMessageID)
	}
	result.CleanupAttemptedMessageIDs = jobs.CompactMessageIDs(cleanupIDs...)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return E2EResetResult{}, err
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)
	draftsDeleted, err := qtx.DeleteDraftSessionsByUser(ctx, userID)
	if err != nil {
		return E2EResetResult{}, err
	}
	productsDeleted, err := qtx.DeleteProductsByUser(ctx, userID)
	if err != nil {
		return E2EResetResult{}, err
	}
	digestsDeleted, err := qtx.DeleteDigestMessagesByUser(ctx, userID)
	if err != nil {
		return E2EResetResult{}, err
	}
	jobsDeleted, err := qtx.DeleteJobsForE2EUser(ctx, sqlcgen.DeleteJobsForE2EUserParams{
		Column1: userID,
		Column2: settings.ChatID,
	})
	if err != nil {
		return E2EResetResult{}, err
	}
	if err := qtx.ResetUserSettingsForE2E(ctx, sqlcgen.ResetUserSettingsForE2EParams{
		UserID:          userID,
		Timezone:        defaultTimezone,
		DigestLocalTime: digestLocalTime,
	}); err != nil {
		return E2EResetResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return E2EResetResult{}, err
	}

	result.Deleted = E2EResetDeletedCounts{
		Drafts:   draftsDeleted,
		Products: productsDeleted,
		Digests:  digestsDeleted,
		Jobs:     jobsDeleted,
	}
	s.logger.InfoContext(ctx, "e2e_reset_completed", observability.LogAttrs(ctx,
		"user_id", userID,
		"chat_id", settings.ChatID,
		"drafts_deleted", draftsDeleted,
		"products_deleted", productsDeleted,
		"digests_deleted", digestsDeleted,
		"jobs_deleted", jobsDeleted,
		"cleanup_attempted_message_count", len(result.CleanupAttemptedMessageIDs),
	)...)
	return result, nil
}

func optionalMessageID(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
