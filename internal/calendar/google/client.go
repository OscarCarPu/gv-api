package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// asAPIError is errors.As specialised, kept private so callers use the Is* helpers.
func asAPIError(err error, target **APIError) bool { return errors.As(err, target) }

// Client is everything the calendar domain needs from Google. It exists so the service can
// be exercised against Fake, which reproduces the behaviour that actually matters here —
// sync tokens going stale, etags failing, grants being revoked — without a network.
//
// Every method takes an access token rather than holding one: tokens are per account, live
// in the database, and are refreshed by the service that owns them.
type Client interface {
	// AuthURL builds the consent URL. state is echoed back to the callback.
	AuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*Token, error)
	RefreshToken(ctx context.Context, refreshToken string) (*Token, error)
	RevokeToken(ctx context.Context, token string) error
	UserEmail(ctx context.Context, accessToken string) (string, error)

	ListCalendars(ctx context.Context, accessToken string) ([]CalendarListEntry, error)

	ListEvents(ctx context.Context, accessToken, calendarID string, p ListEventsParams) (*EventsPage, error)
	GetEvent(ctx context.Context, accessToken, calendarID, eventID string) (*Event, error)
	// ListInstances resolves a series occurrence to the concrete instance id Google wants for
	// a single-instance edit. Constructing that id by string surgery is possible but Google
	// documents the format loosely enough that asking is the safer move.
	ListInstances(ctx context.Context, accessToken, calendarID, eventID, originalStart string) ([]Event, error)
	InsertEvent(ctx context.Context, accessToken, calendarID string, body map[string]any, sendUpdates string) (*Event, error)
	// PatchEvent sends only the given fields. ifMatch is the stored etag: an empty string
	// means "overwrite whatever is there", which this app never does.
	PatchEvent(ctx context.Context, accessToken, calendarID, eventID, ifMatch string, body map[string]any, sendUpdates string) (*Event, error)
	DeleteEvent(ctx context.Context, accessToken, calendarID, eventID, ifMatch, sendUpdates string) error
	MoveEvent(ctx context.Context, accessToken, calendarID, eventID, destination, sendUpdates string) (*Event, error)

	Watch(ctx context.Context, accessToken, calendarID string, req WatchRequest) (*WatchChannel, error)
	StopChannel(ctx context.Context, accessToken, channelID, resourceID string) error
}

// Google's own OAuth endpoints, hardcoded rather than pulled from
// golang.org/x/oauth2/google: that package's only use here would be these two constants, and
// it drags in the GCE metadata client with it.
const (
	defaultAuthURL  = "https://accounts.google.com/o/oauth2/auth"
	defaultTokenURL = "https://oauth2.googleapis.com/token"
)

// Config wires the HTTP client. The three URL fields exist so tests can point the real
// client at an httptest server and check the wire format, not just the fake's behaviour.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string

	CalendarBaseURL string // default https://www.googleapis.com/calendar/v3
	OAuthTokenURL   string // default https://oauth2.googleapis.com/token
	OAuthAuthURL    string // default https://accounts.google.com/o/oauth2/auth
	RevokeURL       string // default https://oauth2.googleapis.com/revoke
	UserInfoURL     string // default https://www.googleapis.com/oauth2/v3/userinfo

	HTTPClient *http.Client
	// MaxAttempts counts the first try. RetryBase is the first backoff step, doubled per
	// attempt with jitter; both are settable so tests do not sleep for real.
	MaxAttempts int
	RetryBase   time.Duration
}

type httpClient struct {
	cfg   Config
	oauth *oauth2.Config
	http  *http.Client
}

func NewClient(cfg Config) Client {
	if cfg.CalendarBaseURL == "" {
		cfg.CalendarBaseURL = "https://www.googleapis.com/calendar/v3"
	}
	if cfg.OAuthTokenURL == "" {
		cfg.OAuthTokenURL = defaultTokenURL
	}
	if cfg.OAuthAuthURL == "" {
		cfg.OAuthAuthURL = defaultAuthURL
	}
	if cfg.RevokeURL == "" {
		cfg.RevokeURL = "https://oauth2.googleapis.com/revoke"
	}
	if cfg.UserInfoURL == "" {
		cfg.UserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 4
	}
	if cfg.RetryBase <= 0 {
		cfg.RetryBase = 500 * time.Millisecond
	}
	return &httpClient{
		cfg:  cfg,
		http: cfg.HTTPClient,
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{ScopeCalendar, ScopeEmail},
			Endpoint:     oauth2.Endpoint{AuthURL: cfg.OAuthAuthURL, TokenURL: cfg.OAuthTokenURL},
		},
	}
}

// --- OAuth ---------------------------------------------------------------------------

/*
AuthURL forces two options that are not defaults and that this app cannot work without:

  - access_type=offline, or Google issues no refresh token and the sync dies the moment the
    first access token expires.
  - prompt=consent, because a refresh token is only handed out on the *first* authorisation.
    Re-connecting an account after a revoke would otherwise return an access token alone and
    look like it worked until an hour later.
*/
func (c *httpClient) AuthURL(state string) string {
	return c.oauth.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	)
}

func (c *httpClient) ExchangeCode(ctx context.Context, code string) (*Token, error) {
	tok, err := c.oauth.Exchange(withHTTPClient(ctx, c.http), code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	return fromOAuth2(tok), nil
}

func (c *httpClient) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	src := c.oauth.TokenSource(withHTTPClient(ctx, c.http), &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	return fromOAuth2(tok), nil
}

func (c *httpClient) RevokeToken(ctx context.Context, token string) error {
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.RevokeURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	// A token Google has already forgotten answers 400. Nothing left to revoke is success.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusBadRequest {
		return parseAPIError(resp.StatusCode, body)
	}
	return nil
}

func (c *httpClient) UserEmail(ctx context.Context, accessToken string) (string, error) {
	var out struct {
		Email string `json:"email"`
	}
	if err := c.do(ctx, accessToken, http.MethodGet, c.cfg.UserInfoURL, nil, "", &out); err != nil {
		return "", err
	}
	if out.Email == "" {
		return "", errors.New("google returned no email for the account")
	}
	return out.Email, nil
}

func fromOAuth2(tok *oauth2.Token) *Token {
	scope, _ := tok.Extra("scope").(string)
	return &Token{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
		Scope:        scope,
	}
}

func withHTTPClient(ctx context.Context, hc *http.Client) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, hc)
}

// IsInvalidGrant reports the one OAuth failure that a human has to fix: the refresh token is
// no longer accepted (revoked, expired, or the consent screen was never published out of
// testing). Retrying it forever is pointless, so the account is parked instead.
func IsInvalidGrant(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorCode == "invalid_grant" {
			return true
		}
		return re.Response != nil && re.Response.StatusCode == http.StatusBadRequest &&
			bytes.Contains(re.Body, []byte("invalid_grant"))
	}
	return false
}

// --- Calendars -----------------------------------------------------------------------

func (c *httpClient) ListCalendars(ctx context.Context, accessToken string) ([]CalendarListEntry, error) {
	var out []CalendarListEntry
	pageToken := ""
	for {
		u := c.url("/users/me/calendarList", url.Values{
			"maxResults":    {"250"},
			"showDeleted":   {"true"},
			"showHidden":    {"true"},
			"minAccessRole": {"freeBusyReader"},
			"pageToken":     {pageToken},
		})
		var page struct {
			Items         []CalendarListEntry `json:"items"`
			NextPageToken string              `json:"nextPageToken"`
		}
		if err := c.do(ctx, accessToken, http.MethodGet, u, nil, "", &page); err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if page.NextPageToken == "" {
			return out, nil
		}
		pageToken = page.NextPageToken
	}
}

// --- Events --------------------------------------------------------------------------

func (c *httpClient) ListEvents(ctx context.Context, accessToken, calendarID string, p ListEventsParams) (*EventsPage, error) {
	if p.MaxResults <= 0 {
		p.MaxResults = 2500
	}
	q := url.Values{
		"maxResults":   {strconv.Itoa(p.MaxResults)},
		"showDeleted":  {strconv.FormatBool(p.ShowDeleted)},
		"singleEvents": {strconv.FormatBool(p.SingleEvents)},
	}
	if p.SyncToken != "" {
		q.Set("syncToken", p.SyncToken)
	}
	if p.PageToken != "" {
		q.Set("pageToken", p.PageToken)
	}
	var page EventsPage
	if err := c.do(ctx, accessToken, http.MethodGet,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events", q), nil, "", &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *httpClient) GetEvent(ctx context.Context, accessToken, calendarID, eventID string) (*Event, error) {
	var ev Event
	if err := c.do(ctx, accessToken, http.MethodGet,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), nil),
		nil, "", &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func (c *httpClient) ListInstances(ctx context.Context, accessToken, calendarID, eventID, originalStart string) ([]Event, error) {
	q := url.Values{"maxResults": {"250"}, "showDeleted": {"true"}}
	if originalStart != "" {
		q.Set("originalStart", originalStart)
	}
	var page EventsPage
	if err := c.do(ctx, accessToken, http.MethodGet,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID)+"/instances", q),
		nil, "", &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (c *httpClient) InsertEvent(ctx context.Context, accessToken, calendarID string, body map[string]any, sendUpdates string) (*Event, error) {
	var ev Event
	if err := c.do(ctx, accessToken, http.MethodPost,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events", sendUpdatesQuery(sendUpdates)),
		body, "", &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func (c *httpClient) PatchEvent(ctx context.Context, accessToken, calendarID, eventID, ifMatch string, body map[string]any, sendUpdates string) (*Event, error) {
	var ev Event
	if err := c.do(ctx, accessToken, http.MethodPatch,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), sendUpdatesQuery(sendUpdates)),
		body, ifMatch, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func (c *httpClient) DeleteEvent(ctx context.Context, accessToken, calendarID, eventID, ifMatch, sendUpdates string) error {
	return c.do(ctx, accessToken, http.MethodDelete,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), sendUpdatesQuery(sendUpdates)),
		nil, ifMatch, nil)
}

func (c *httpClient) MoveEvent(ctx context.Context, accessToken, calendarID, eventID, destination, sendUpdates string) (*Event, error) {
	q := sendUpdatesQuery(sendUpdates)
	q.Set("destination", destination)
	var ev Event
	if err := c.do(ctx, accessToken, http.MethodPost,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID)+"/move", q),
		nil, "", &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func sendUpdatesQuery(sendUpdates string) url.Values {
	q := url.Values{}
	if sendUpdates != "" {
		q.Set("sendUpdates", sendUpdates)
	}
	return q
}

// --- Push ----------------------------------------------------------------------------

func (c *httpClient) Watch(ctx context.Context, accessToken, calendarID string, req WatchRequest) (*WatchChannel, error) {
	body := map[string]any{
		"id":      req.ID,
		"type":    "web_hook",
		"address": req.Address,
		"token":   req.Token,
	}
	if req.TTL > 0 {
		body["params"] = map[string]string{"ttl": strconv.Itoa(int(req.TTL.Seconds()))}
	}
	var out channelResponse
	if err := c.do(ctx, accessToken, http.MethodPost,
		c.url("/calendars/"+url.PathEscape(calendarID)+"/events/watch", nil), body, "", &out); err != nil {
		return nil, err
	}
	ch := out.toChannel()
	return &ch, nil
}

func (c *httpClient) StopChannel(ctx context.Context, accessToken, channelID, resourceID string) error {
	body := map[string]any{"id": channelID, "resourceId": resourceID}
	err := c.do(ctx, accessToken, http.MethodPost, c.url("/channels/stop", nil), body, "", nil)
	// A channel Google has already dropped is not an error worth propagating: the caller is
	// tidying up after a replacement it already created.
	if err != nil && (IsNotFound(err) || statusIs(err, 400)) {
		return nil
	}
	return err
}

// --- Transport -----------------------------------------------------------------------

func (c *httpClient) url(path string, q url.Values) string {
	u := c.cfg.CalendarBaseURL + path
	if len(q) > 0 {
		// Empty values are dropped so an unset page token does not turn into pageToken=.
		for k, v := range q {
			if len(v) == 0 || v[0] == "" {
				q.Del(k)
			}
		}
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
	}
	return u
}

/*
do performs one request, retrying the failures that are worth retrying.

Retried: rate limits (403 with a usageLimits reason, or 429) and 5xx, per Google's own
guidance — exponential backoff with jitter, honouring Retry-After when present. Everything
else, including 410 and 412, returns immediately: those are answers, not failures, and the
caller has specific work to do for each.
*/
func (c *httpClient) do(ctx context.Context, accessToken, method, u string, body any, ifMatch string, out any) error {
	var lastErr error
	for attempt := 0; attempt < c.cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			wait := c.backoff(attempt, lastErr)
			slog.DebugContext(ctx, "google api retry", "attempt", attempt, "wait", wait, "url", u)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		var reader io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			// Transport failures are worth one more go: a tunnel hiccup is not a verdict.
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= 300 {
			apiErr := parseAPIError(resp.StatusCode, raw)
			apiErr.Reason = firstNonEmpty(apiErr.Reason, resp.Header.Get("X-Goog-Error-Reason"))
			if retryable(apiErr) {
				lastErr = apiErr
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, convErr := strconv.Atoi(ra); convErr == nil {
						select {
						case <-time.After(time.Duration(secs) * time.Second):
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
				continue
			}
			return apiErr
		}

		if out == nil || len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode google response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("google api: giving up after %d attempts: %w", c.cfg.MaxAttempts, lastErr)
}

func retryable(err *APIError) bool {
	return err.Status >= 500 || IsRateLimited(err)
}

func (c *httpClient) backoff(attempt int, _ error) time.Duration {
	step := c.cfg.RetryBase * time.Duration(1<<(attempt-1))
	// Jitter keeps four accounts syncing on the same tick from retrying in lockstep.
	jitter := time.Duration(rand.Int64N(int64(c.cfg.RetryBase)))
	return step + jitter
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
