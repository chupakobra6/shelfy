package postgres

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
)

type UserSettings struct {
	UserID             int64
	ChatID             int64
	Timezone           string
	DigestLocalTime    string
	DashboardMessageID *int64
}

func (s *Store) UpsertUserSettings(ctx context.Context, settings UserSettings) error {
	_, err := s.queries.UpsertUserSettings(ctx, sqlcgen.UpsertUserSettingsParams{
		UserID:             settings.UserID,
		ChatID:             settings.ChatID,
		Timezone:           settings.Timezone,
		DigestLocalTime:    settings.DigestLocalTime,
		DashboardMessageID: settings.DashboardMessageID,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "user_settings_upserted", observability.LogAttrs(ctx,
			"user_id", settings.UserID,
			"chat_id", settings.ChatID,
			"timezone", settings.Timezone,
			"digest_local_time", settings.DigestLocalTime,
			"dashboard_message_id", messageIDValue(settings.DashboardMessageID),
		)...)
	}
	return err
}

func (s *Store) SetDashboardMessageID(ctx context.Context, userID, messageID int64) error {
	err := s.queries.SetDashboardMessageID(ctx, sqlcgen.SetDashboardMessageIDParams{
		UserID:             userID,
		DashboardMessageID: &messageID,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "dashboard_message_id_set", observability.LogAttrs(ctx,
			"user_id", userID,
			"dashboard_message_id", messageID,
		)...)
	}
	return err
}

func (s *Store) UpdateUserTimezone(ctx context.Context, userID int64, timezone string) error {
	err := s.queries.UpdateUserTimezone(ctx, sqlcgen.UpdateUserTimezoneParams{
		UserID:   userID,
		Timezone: timezone,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "user_timezone_updated", observability.LogAttrs(ctx, "user_id", userID, "timezone", timezone)...)
	}
	return err
}

func (s *Store) UpdateUserDigestLocalTime(ctx context.Context, userID int64, digestLocalTime string) error {
	err := s.queries.UpdateUserDigestLocalTime(ctx, sqlcgen.UpdateUserDigestLocalTimeParams{
		UserID:          userID,
		DigestLocalTime: digestLocalTime,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "user_digest_local_time_updated", observability.LogAttrs(ctx, "user_id", userID, "digest_local_time", digestLocalTime)...)
	}
	return err
}

func (s *Store) GetUserSettings(ctx context.Context, userID int64) (UserSettings, error) {
	row, err := s.queries.GetUserSettings(ctx, userID)
	if err != nil {
		return UserSettings{}, err
	}
	return newUserSettings(row.UserID, row.ChatID, row.Timezone, row.DigestLocalTime, row.DashboardMessageID), nil
}

func (s *Store) ListUsers(ctx context.Context) ([]UserSettings, error) {
	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]UserSettings, 0, len(rows))
	for _, row := range rows {
		result = append(result, newUserSettings(row.UserID, row.ChatID, row.Timezone, row.DigestLocalTime, row.DashboardMessageID))
	}
	return result, nil
}

func (s *Store) DashboardStats(ctx context.Context, userID int64, now time.Time) (domain.DashboardStats, error) {
	soon := now.AddDate(0, 0, 3)
	row, err := s.queries.DashboardStats(ctx, sqlcgen.DashboardStatsParams{
		UserID:  userID,
		Column2: pgDateFromTime(now),
		Column3: pgDateFromTime(soon),
	})
	if err != nil {
		return domain.DashboardStats{}, err
	}
	return domain.DashboardStats{
		ActiveCount:    int(row.ActiveCount),
		SoonCount:      int(row.SoonCount),
		ExpiredCount:   int(row.ExpiredCount),
		ConsumedCount:  int(row.ConsumedCount),
		DiscardedCount: int(row.DiscardedCount),
		DeletedCount:   int(row.DeletedCount),
	}, nil
}

func messageIDValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func newUserSettings(userID, chatID int64, timezone, digestLocalTime string, dashboardMessageID *int64) UserSettings {
	return UserSettings{
		UserID:             userID,
		ChatID:             chatID,
		Timezone:           timezone,
		DigestLocalTime:    digestLocalTime,
		DashboardMessageID: dashboardMessageID,
	}
}
