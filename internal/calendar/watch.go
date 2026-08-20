package calendar

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"gv-api/internal/calendar/google"
)

/*
EnsureWatches keeps a push channel alive for every synced calendar.

Google's channels expire and there is no renewal call: the only way to keep one is to create
a replacement before the old one dies and then stop the old one. That makes this a
housekeeping job rather than a one-off setup step, and it is the single point of failure for
"changes show up in seconds" — if it stops working, everything still works, just slower, via
the poll. That is why every failure here is logged loudly and surfaced in the sync status
instead of being swallowed.
*/
func (s *Service) EnsureWatches(ctx context.Context) error {
	if !s.Configured() || !s.cfg.WebhookEnabled || s.cfg.WebhookURL == "" {
		return nil
	}
	due, err := s.repo.ListCalendarsNeedingWatch(ctx, s.now().Add(s.cfg.WatchRenewBefore))
	if err != nil {
		return err
	}
	for _, cal := range due {
		if err := s.renewWatch(ctx, cal); err != nil {
			slog.ErrorContext(ctx, "calendar: could not establish push channel",
				"calendar", cal.ID, "summary", cal.Summary, "error", err)
			// Not fatal, and not retried in a tight loop: the next pass tries again, and the
			// poll covers the gap in the meantime.
			continue
		}
	}
	return nil
}

func (s *Service) renewWatch(ctx context.Context, cal CalendarRecord) error {
	acc, err := s.repo.GetAccount(ctx, cal.AccountID)
	if err != nil {
		return err
	}
	token, err := s.accessTokenFor(ctx, acc)
	if err != nil {
		return err
	}

	channelID, err := randomHex(16)
	if err != nil {
		return err
	}
	// The channel token is the webhook's only credential: Google echoes it back in
	// X-Goog-Channel-Token, and the endpoint has to be public because Google cannot
	// authenticate itself any other way.
	channelToken, err := randomHex(24)
	if err != nil {
		return err
	}

	ch, err := s.gc.Watch(ctx, token, cal.GoogleCalendarID, google.WatchRequest{
		ID:      channelID,
		Token:   channelToken,
		Address: s.cfg.WebhookURL,
		TTL:     s.cfg.WatchTTL,
	})
	if err != nil {
		return err
	}

	previousID, previousResource := cal.WatchChannelID, cal.WatchResourceID
	if err := s.repo.SetCalendarWatch(ctx, cal.ID, WatchInfo{
		ChannelID:  ch.ID,
		ResourceID: ch.ResourceID,
		Token:      channelToken,
		ExpiresAt:  ch.Expiration,
	}); err != nil {
		return err
	}

	// Only now is the old channel dropped: if storing the new one had failed, the old one
	// would still be delivering and nothing would have gone quiet.
	if previousID != nil && previousResource != nil && *previousID != ch.ID {
		if err := s.gc.StopChannel(ctx, token, *previousID, *previousResource); err != nil {
			slog.WarnContext(ctx, "calendar: stopping the replaced channel",
				"calendar", cal.ID, "channel", *previousID, "error", err)
		}
	}
	slog.InfoContext(ctx, "calendar: push channel established",
		"calendar", cal.ID, "summary", cal.Summary, "expires", ch.Expiration)
	return nil
}

// stopWatch tears a channel down and forgets it. Used when a calendar stops being synced or
// its account is disconnected.
func (s *Service) stopWatch(ctx context.Context, cal CalendarRecord) {
	if cal.WatchChannelID == nil || cal.WatchResourceID == nil {
		return
	}
	acc, err := s.repo.GetAccount(ctx, cal.AccountID)
	if err == nil {
		if token, tokErr := s.accessTokenFor(ctx, acc); tokErr == nil {
			if err := s.gc.StopChannel(ctx, token, *cal.WatchChannelID, *cal.WatchResourceID); err != nil {
				slog.WarnContext(ctx, "calendar: stopping channel", "calendar", cal.ID, "error", err)
			}
		}
	}
	if err := s.repo.ClearCalendarWatch(ctx, cal.ID); err != nil {
		slog.ErrorContext(ctx, "calendar: clearing channel", "calendar", cal.ID, "error", err)
	}
}

/*
HandleWebhook accepts a push notification.

It does as little as possible: match the channel, check the token, queue the calendar. Google
retries a notification that is not answered promptly and eventually kills a channel that keeps
timing out, so the sync itself must not happen on this path.

The body is empty and unauthenticated by design — everything is in the headers — so the token
comparison is the whole of the security here, and it is constant-time.
*/
func (s *Service) HandleWebhook(ctx context.Context, channelID, resourceState, channelToken string) error {
	if channelID == "" {
		return fmt.Errorf("%w: missing channel id", ErrNotFound)
	}
	cal, err := s.repo.GetCalendarByChannel(ctx, channelID)
	if err != nil {
		// An unknown channel is usually one we replaced and Google has not caught up with, or
		// one left over from a previous database. Nothing to do, and nothing alarming.
		slog.DebugContext(ctx, "calendar: notification for an unknown channel", "channel", channelID)
		return ErrNotFound
	}
	if cal.WatchToken == nil || !hmac.Equal([]byte(*cal.WatchToken), []byte(channelToken)) {
		slog.WarnContext(ctx, "calendar: notification with a bad channel token", "channel", channelID)
		return ErrInvalidState
	}
	// "sync" is the handshake Google sends when a channel is created; there is nothing new
	// behind it.
	if resourceState == "sync" {
		return nil
	}
	s.notifyChange(cal.ID)
	return nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Subscribe hands out a stream of change notifications for the SSE endpoint.
func (s *Service) Subscribe() (<-chan StreamMessage, func()) { return s.stream.Subscribe() }

// WatchTTL and friends are read by the worker.
func (s *Service) WatchRenewBefore() time.Duration { return s.cfg.WatchRenewBefore }
