package bot

import (
	"context"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) handleSettingsCallback(ctx context.Context, callback telegram.CallbackQuery, parts []string) error {
	if callback.Message == nil || len(parts) < 2 {
		return nil
	}
	settings, err := s.store.GetUserSettings(ctx, callback.From.ID)
	if err != nil {
		return err
	}
	switch parts[1] {
	case "digest":
		if len(parts) < 3 {
			return nil
		}
		updated, err := adjustDigestTime(settings.DigestLocalTime, parts[2])
		if err != nil {
			return err
		}
		if err := s.store.UpdateUserDigestLocalTime(ctx, callback.From.ID, updated); err != nil {
			return err
		}
		settings.DigestLocalTime = updated
		s.logger.InfoContext(ctx, "settings_updated", observability.LogAttrs(ctx,
			"field", "digest_local_time",
			"value", updated,
		)...)
	case "timezone":
		if len(parts) < 3 {
			return nil
		}
		if !isAllowedTimezone(parts[2]) {
			return nil
		}
		if err := s.store.UpdateUserTimezone(ctx, callback.From.ID, parts[2]); err != nil {
			return err
		}
		settings.Timezone = parts[2]
		s.logger.InfoContext(ctx, "settings_updated", observability.LogAttrs(ctx,
			"field", "timezone",
			"value", parts[2],
		)...)
	default:
		return nil
	}
	return s.applyDashboardCallbackState(ctx, callback, settingsDashboardState(), "dashboard_transition_applied", "trigger", callback.Data)
}
