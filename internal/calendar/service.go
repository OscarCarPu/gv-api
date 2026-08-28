package calendar

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"gv-api/internal/calendar/google"
)

// Config is everything the calendar service needs from the environment.
type Config struct {
	// ClientID/ClientSecret empty means the domain runs read-only-and-empty: it mounts, it
	// answers, and every connect or sync attempt says it is not configured. The API must
	// still boot without Google credentials, the same way it boots without a Bluetooth radio.
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// WebAppURL is where the OAuth callback sends the browser back to. The callback lands on
	// the API host (that is what Google was given), but the user is looking at gv-web.
	WebAppURL string

	WebhookEnabled   bool
	WebhookURL       string
	WatchTTL         time.Duration
	WatchRenewBefore time.Duration
	SyncInterval     time.Duration
	Debounce         time.Duration
	// StateSecret signs the OAuth state parameter. Reusing JWT_SECRET is deliberate: it is
	// already the app's "this came from us" key.
	StateSecret []byte
	TokenKey    string
	// CancelledRetention is how long a cancelled one-off event is kept before being purged.
	CancelledRetention time.Duration
}

// Service holds the calendar domain's rules.
type Service struct {
	repo   Repository
	gc     google.Client
	cipher *tokenCipher
	cfg    Config
	stream *Stream
	loc    *time.Location

	// changes carries calendar ids that a webhook says have moved. The worker debounces and
	// drains it; nothing here blocks on a sync, because Google drops a channel that does not
	// answer its POST quickly.
	changes chan int32

	// One sync at a time per calendar. A push notification and the poll tick landing together
	// would otherwise spend the same sync token twice and turn a good token into a 410.
	syncMu  sync.Mutex
	syncing map[int32]bool

	now func() time.Time
}

func NewService(repo Repository, gc google.Client, cfg Config, loc *time.Location) (*Service, error) {
	if cfg.WatchTTL <= 0 {
		cfg.WatchTTL = 7 * 24 * time.Hour
	}
	if cfg.WatchRenewBefore <= 0 {
		cfg.WatchRenewBefore = 24 * time.Hour
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 15 * time.Minute
	}
	if cfg.Debounce <= 0 {
		cfg.Debounce = 2 * time.Second
	}
	if cfg.CancelledRetention <= 0 {
		cfg.CancelledRetention = 90 * 24 * time.Hour
	}
	if loc == nil {
		loc = time.UTC
	}

	s := &Service{
		repo:    repo,
		gc:      gc,
		cfg:     cfg,
		stream:  NewStream(),
		loc:     loc,
		changes: make(chan int32, 256),
		syncing: map[int32]bool{},
		now:     time.Now,
	}

	// Without a key the tokens would have to be stored in the clear, which is not a trade
	// this feature gets to make silently: it refuses to configure instead.
	if cfg.ClientID != "" && cfg.ClientSecret != "" {
		if cfg.TokenKey == "" {
			return nil, fmt.Errorf("GOOGLE_TOKEN_KEY is required when google oauth is configured")
		}
		c, err := newTokenCipher(cfg.TokenKey)
		if err != nil {
			return nil, err
		}
		s.cipher = c
	}
	return s, nil
}

// Configured reports whether Google credentials are present.
func (s *Service) Configured() bool { return s.cipher != nil }

// Stream exposes the SSE hub so the handler can subscribe clients to it.
func (s *Service) Stream() *Stream { return s.stream }

// Changes is the queue of calendars a webhook reported as changed. The worker owns it; tests
// read it to assert that a notification was accepted.
func (s *Service) Changes() <-chan int32 { return s.changes }

// Interval is the poll period, which the worker needs and the status endpoint reports.
func (s *Service) Interval() time.Duration { return s.cfg.SyncInterval }

func (s *Service) requireConfigured() error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	return nil
}

// --- OAuth ---------------------------------------------------------------------------

/*
oauthStateTTL is how long a consent URL stays usable.

Long enough to connect several accounts in one sitting — the URL is not single-use, so the same
one serves each account in turn — and short enough that a link left in a chat log or a shell
history is not an open door for long.
*/
const oauthStateTTL = 30 * time.Minute

/*
AuthURL returns the Google consent URL for adding an account.

The state parameter is an HMAC over a nonce and an expiry rather than a row in a table: it is
single-use by virtue of being short-lived (10 minutes), it needs no cleanup, and it survives
an API restart in the middle of a consent flow, which a value held in memory would not.
*/
func (s *Service) AuthURL(_ context.Context) (AuthURLResponse, error) {
	if err := s.requireConfigured(); err != nil {
		return AuthURLResponse{}, err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return AuthURLResponse{}, err
	}
	payload := fmt.Sprintf("%s.%d", hex.EncodeToString(nonce), s.now().Add(oauthStateTTL).Unix())
	state := payload + "." + s.signState(payload)
	return AuthURLResponse{URL: s.gc.AuthURL(state)}, nil
}

func (s *Service) signState(payload string) string {
	mac := hmac.New(sha256.New, s.cfg.StateSecret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) verifyState(state string) error {
	parts := strings.Split(state, ".")
	if len(parts) != 3 {
		return ErrInvalidState
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(s.signState(payload)), []byte(parts[2])) {
		return ErrInvalidState
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || s.now().After(time.Unix(exp, 0)) {
		return ErrInvalidState
	}
	return nil
}

/*
HandleCallback finishes the consent flow: it exchanges the code, stores the grant, and pulls
the account's calendar list so the user immediately sees what there is to sync.

It returns where to send the browser. The callback is hit by Google's redirect on the API
host, where there is no session cookie and nothing to render, so the only sensible answer is
a redirect back to the web app.
*/
func (s *Service) HandleCallback(ctx context.Context, code, state string) (string, error) {
	if err := s.requireConfigured(); err != nil {
		return "", err
	}
	if err := s.verifyState(state); err != nil {
		return "", err
	}
	if code == "" {
		return "", fmt.Errorf("%w: no code", ErrInvalidState)
	}

	tok, err := s.gc.ExchangeCode(ctx, code)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if tok.RefreshToken == "" {
		// Without a refresh token the grant is useless in an hour. This is what happens when
		// the consent screen is re-approved without prompt=consent, so say so plainly.
		return "", fmt.Errorf("%w: google returned no refresh token; revoke the app's access at myaccount.google.com and connect again", ErrUpstream)
	}

	email, err := s.gc.UserEmail(ctx, tok.AccessToken)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	sealedRefresh, err := s.cipher.seal(tok.RefreshToken)
	if err != nil {
		return "", err
	}
	sealedAccess, err := s.cipher.seal(tok.AccessToken)
	if err != nil {
		return "", err
	}
	expiry := tok.Expiry
	acc, err := s.repo.UpsertAccount(ctx, UpsertAccountParams{
		Email:                email,
		RefreshTokenSealed:   sealedRefresh,
		AccessTokenSealed:    sealedAccess,
		AccessTokenExpiresAt: &expiry,
		Scopes:               tok.Scope,
	})
	if err != nil {
		return "", err
	}

	if err := s.importCalendars(ctx, acc, tok.AccessToken); err != nil {
		// The account is connected; a failed calendar list is a transient upstream problem
		// that the next sync pass fixes. Do not lose the grant over it.
		slog.ErrorContext(ctx, "calendar: importing calendar list after connect", "account", email, "error", err)
	}
	s.stream.Publish(StreamMessage{Type: "account.connected", AccountEmail: email})

	return s.redirectTo("connected", email), nil
}

func (s *Service) redirectTo(status, email string) string {
	base := strings.TrimSuffix(s.cfg.WebAppURL, "/")
	if base == "" {
		return "/"
	}
	q := url.Values{status: {email}}
	return base + "/calendar?" + q.Encode()
}

// importCalendars refreshes an account's calendar list. Calendars that vanished from Google
// are marked deleted rather than removed: their events stay visible, and a shared calendar
// that comes back keeps its local preferences.
func (s *Service) importCalendars(ctx context.Context, acc AccountRecord, accessToken string) error {
	entries, err := s.gc.ListCalendars(ctx, accessToken)
	if err != nil {
		return err
	}
	seen := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Deleted {
			continue
		}
		seen = append(seen, e.ID)
		if _, err := s.repo.UpsertCalendar(ctx, UpsertCalendarParams{
			AccountID:        acc.ID,
			GoogleCalendarID: e.ID,
			Summary:          e.Name(),
			Description:      e.Description,
			TimeZone:         firstNonEmpty(e.TimeZone, s.loc.String()),
			BackgroundColor:  e.BackgroundColor,
			ForegroundColor:  e.ForegroundColor,
			AccessRole:       e.AccessRole,
			IsPrimary:        e.Primary,
			// Holiday and birthday calendars are enormous and never interesting here, and the
			// initial import cannot be bounded by date, so they start switched off.
			SyncEnabled: defaultSyncEnabled(e),
		}); err != nil {
			return err
		}
	}
	return s.repo.MarkCalendarsDeleted(ctx, acc.ID, seen)
}

// defaultSyncEnabled decides whether a newly discovered calendar is synced without being
// asked. Only applies the first time a calendar is seen; after that the user's choice wins.
func defaultSyncEnabled(e google.CalendarListEntry) bool {
	id := strings.ToLower(e.ID)
	switch {
	case strings.Contains(id, "#holiday@group.v.calendar.google.com"),
		strings.Contains(id, "#contacts@group.v.calendar.google.com"),
		strings.Contains(id, "#weeknum@group.v.calendar.google.com"):
		return false
	}
	return true
}

// --- accounts ------------------------------------------------------------------------

func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(accounts))
	for _, a := range accounts {
		cals, err := s.repo.ListCalendarsByAccount(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, toAccountDTO(a, len(cals)))
	}
	return out, nil
}

func toAccountDTO(a AccountRecord, calendars int) Account {
	return Account{
		ID:            a.ID,
		Email:         a.Email,
		Label:         a.Label,
		Color:         a.Color,
		Status:        a.Status,
		Calendars:     calendars,
		LastSyncAt:    a.LastSyncAt,
		LastSyncError: a.LastSyncError,
		CreatedAt:     a.CreatedAt,
	}
}

func (s *Service) UpdateAccount(ctx context.Context, id int32, req UpdateAccountRequest) (Account, error) {
	acc, err := s.repo.UpdateAccountMeta(ctx, id, req.Label, req.Color)
	if err != nil {
		return Account{}, err
	}
	cals, err := s.repo.ListCalendarsByAccount(ctx, id)
	if err != nil {
		return Account{}, err
	}
	return toAccountDTO(acc, len(cals)), nil
}

/*
DeleteAccount disconnects an account: it stops the push channels, revokes the grant at Google
and deletes the rows (calendars and events go with it, by cascade).

Revoking first is deliberate. Dropping the rows while the grant lives would leave an
authorisation nobody can see in the app but that still appears on the user's Google account
page, which is exactly the kind of leftover access that is impossible to reason about later.
*/
func (s *Service) DeleteAccount(ctx context.Context, id int32) error {
	acc, err := s.repo.GetAccount(ctx, id)
	if err != nil {
		return err
	}

	if s.Configured() {
		if token, tokErr := s.accessTokenFor(ctx, acc); tokErr == nil {
			cals, listErr := s.repo.ListCalendarsByAccount(ctx, id)
			if listErr == nil {
				for _, c := range cals {
					if c.WatchChannelID != nil && c.WatchResourceID != nil {
						if err := s.gc.StopChannel(ctx, token, *c.WatchChannelID, *c.WatchResourceID); err != nil {
							slog.WarnContext(ctx, "calendar: stopping channel on disconnect",
								"calendar", c.ID, "error", err)
						}
					}
				}
			}
		}
		if refresh, opErr := s.cipher.open(acc.RefreshTokenSealed); opErr == nil && refresh != "" {
			if err := s.gc.RevokeToken(ctx, refresh); err != nil {
				slog.WarnContext(ctx, "calendar: revoking grant", "account", acc.Email, "error", err)
			}
		}
	}

	if err := s.repo.DeleteAccount(ctx, id); err != nil {
		return err
	}
	// Deliberately at info: this drops every mirrored event of the account, and the only other
	// trace it leaves is an absence.
	slog.InfoContext(ctx, "calendar: account disconnected, its calendars and events are gone",
		"account", acc.ID, "email", acc.Email)
	s.stream.Publish(StreamMessage{Type: "account.disconnected", AccountEmail: acc.Email})
	return nil
}

/*
accessTokenFor returns a usable access token for the account, refreshing it when needed.

The cached token is used until a minute before it expires; the margin exists because a token
that expires mid-sync turns a clean pass into a 401 halfway through, and re-listing a
calendar is more expensive than refreshing early.

invalid_grant is the one failure that is not retried: the grant is gone and no amount of
trying brings it back, so the account is parked as needs_reauth and the user is told.
*/
func (s *Service) accessTokenFor(ctx context.Context, acc AccountRecord) (string, error) {
	if err := s.requireConfigured(); err != nil {
		return "", err
	}
	if acc.Status != "connected" {
		return "", ErrNeedsReauth
	}
	if acc.AccessTokenSealed != nil && acc.AccessTokenExpiresAt != nil &&
		acc.AccessTokenExpiresAt.After(s.now().Add(time.Minute)) {
		if token, err := s.cipher.open(acc.AccessTokenSealed); err == nil && token != "" {
			return token, nil
		}
	}

	refresh, err := s.cipher.open(acc.RefreshTokenSealed)
	if err != nil {
		return "", err
	}
	tok, err := s.gc.RefreshToken(ctx, refresh)
	if err != nil {
		if google.IsInvalidGrant(err) {
			msg := "google rejected the stored grant (invalid_grant); reconnect this account"
			if statusErr := s.repo.UpdateAccountStatus(ctx, acc.ID, "needs_reauth", &msg); statusErr != nil {
				slog.ErrorContext(ctx, "calendar: parking account", "account", acc.Email, "error", statusErr)
			}
			s.stream.Publish(StreamMessage{Type: "account.needs_reauth", AccountEmail: acc.Email})
			return "", ErrNeedsReauth
		}
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	sealed, err := s.cipher.seal(tok.AccessToken)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateAccountAccessToken(ctx, acc.ID, sealed, tok.Expiry); err != nil {
		// Not fatal: the token in hand works, it just will not survive a restart.
		slog.WarnContext(ctx, "calendar: caching access token", "account", acc.Email, "error", err)
	}
	return tok.AccessToken, nil
}

// --- calendars -----------------------------------------------------------------------

func (s *Service) ListCalendars(ctx context.Context) ([]Calendar, error) {
	views, err := s.repo.ListCalendars(ctx)
	if err != nil {
		return nil, err
	}
	assignColors(views)
	out := make([]Calendar, 0, len(views))
	for _, v := range views {
		out = append(out, toCalendarDTO(v))
	}
	return out, nil
}

func toCalendarDTO(v CalendarView) Calendar {
	return Calendar{
		ID:               v.ID,
		AccountID:        v.AccountID,
		AccountEmail:     v.AccountEmail,
		AccountStatus:    v.AccountStatus,
		GoogleCalendarID: v.GoogleCalendarID,
		Summary:          v.Summary,
		Description:      v.Description,
		TimeZone:         v.TimeZone,
		Color:            v.DisplayColor(),
		BackgroundColor:  v.BackgroundColor,
		ForegroundColor:  v.ForegroundColor,
		AccessRole:       v.AccessRole,
		Writable:         v.Writable(),
		IsPrimary:        v.IsPrimary,
		SyncEnabled:      v.SyncEnabled,
		Visible:          v.Visible,
		Deleted:          v.DeletedAt != nil,
		Sync: CalendarSyncState{
			HasSyncToken:   v.SyncToken != nil,
			LastSyncAt:     v.LastSyncAt,
			LastFullSyncAt: v.LastFullSyncAt,
			LastSyncError:  v.LastSyncError,
			WatchActive:    v.WatchChannelID != nil,
			WatchExpiresAt: v.WatchExpiresAt,
		},
	}
}

// DisplayColor is the colour clients paint with. An override is the user's explicit choice, the
// assignment is gv's, and Google's own value is only a last resort — it is the same pale cyan for
// every primary calendar, so it identifies nothing.
func (v CalendarView) DisplayColor() string {
	if v.ColorOverride != "" {
		return v.ColorOverride
	}
	if v.AssignedColor != "" {
		return v.AssignedColor
	}
	return v.BackgroundColor
}

func (s *Service) UpdateCalendar(ctx context.Context, id int32, req UpdateCalendarRequest) (Calendar, error) {
	before, err := s.repo.GetCalendar(ctx, id)
	if err != nil {
		return Calendar{}, err
	}
	rec, err := s.repo.UpdateCalendarPrefs(ctx, id, CalendarPrefs{
		SyncEnabled:   req.SyncEnabled,
		Visible:       req.Visible,
		ColorOverride: req.ColorOverride,
	})
	if err != nil {
		return Calendar{}, err
	}

	// Turning a calendar off should not leave its events sitting in the local copy pretending
	// to be current, and it should not leave a push channel alive either.
	if before.SyncEnabled && !rec.SyncEnabled {
		if _, err := s.repo.DeleteEventsForCalendar(ctx, id); err != nil {
			slog.ErrorContext(ctx, "calendar: clearing events of disabled calendar", "calendar", id, "error", err)
		}
		if err := s.repo.ClearCalendarSyncToken(ctx, id); err != nil {
			slog.ErrorContext(ctx, "calendar: clearing sync token", "calendar", id, "error", err)
		}
		s.stopWatch(ctx, rec)
	}

	views, err := s.repo.ListCalendars(ctx)
	if err != nil {
		return Calendar{}, err
	}
	for _, v := range views {
		if v.ID == id {
			return toCalendarDTO(v), nil
		}
	}
	return Calendar{}, ErrNotFound
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func jsonOrEmpty(v any, fallback string) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte(fallback)
	}
	return raw
}

// SetNow overrides the clock. Exported for tests that need to look at what happens either side
// of the consent window; nothing in the app calls it.
func (s *Service) SetNow(now func() time.Time) { s.now = now }
