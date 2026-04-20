package postgres

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

func (s *Store) CurrentNow(ctx context.Context, fallback time.Time) (time.Time, error) {
	override, err := s.queries.GetClockOverride(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if override.Valid {
		return timeFromPgTimestamptz(override), nil
	}
	return fallback.UTC(), nil
}

func (s *Store) SetClockOverride(ctx context.Context, value *time.Time) error {
	err := s.queries.SetClockOverride(ctx, pgTimestamptzFromTimePtr(value))
	if err == nil {
		override := ""
		if value != nil {
			override = value.UTC().Format(time.RFC3339)
		}
		s.logger.InfoContext(ctx, "clock_override_set", observability.LogAttrs(ctx, "override_now", override)...)
	}
	return err
}

func (s *Store) AdvanceClock(ctx context.Context, delta time.Duration) (time.Time, error) {
	current, err := s.CurrentNow(ctx, time.Now().UTC())
	if err != nil {
		return time.Time{}, err
	}
	next := current.Add(delta)
	if err := s.SetClockOverride(ctx, &next); err != nil {
		return time.Time{}, err
	}
	s.logger.InfoContext(ctx, "clock_advanced", observability.LogAttrs(ctx, "delta", delta.String(), "now", next.UTC().Format(time.RFC3339))...)
	return next, nil
}
