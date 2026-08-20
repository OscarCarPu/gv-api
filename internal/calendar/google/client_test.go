package google_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"gv-api/internal/calendar/google"

	"github.com/stretchr/testify/require"
)

// The fake covers behaviour; these tests cover the wire. What Google is sent and how its
// answers are read is the part a fake cannot check.

func newTestClient(t *testing.T, handler http.Handler) (google.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return google.NewClient(google.Config{
		ClientID:        "client-id",
		ClientSecret:    "secret",
		RedirectURL:     "https://api.example/calendar/google/callback",
		CalendarBaseURL: srv.URL,
		OAuthTokenURL:   srv.URL + "/token",
		RevokeURL:       srv.URL + "/revoke",
		UserInfoURL:     srv.URL + "/userinfo",
		MaxAttempts:     3,
		RetryBase:       time.Millisecond,
	}), srv
}

func TestClient_AuthURL_ForcesOfflineAndConsent(t *testing.T) {
	c, _ := newTestClient(t, http.NotFoundHandler())
	url := c.AuthURL("state-123")
	require.Contains(t, url, "access_type=offline",
		"without it google issues no refresh token and the sync dies in an hour")
	require.Contains(t, url, "prompt=consent",
		"without it a re-connect returns no refresh token at all")
	require.Contains(t, url, "state=state-123")
	require.Contains(t, url, "scope=")
	require.Contains(t, url, "calendar")
}

func TestClient_ListEvents_SendsTheParametersSyncRequires(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":         []map[string]any{{"id": "a", "summary": "One"}},
			"nextSyncToken": "tok-1",
		})
	}))

	page, err := c.ListEvents(context.Background(), "access", "me@example.com", google.ListEventsParams{
		SyncToken: "prev-token", MaxResults: 2500, ShowDeleted: true, SingleEvents: false,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "tok-1", page.NextSyncToken)

	require.Equal(t, "Bearer access", got.Header.Get("Authorization"))
	q := got.URL.Query()
	require.Equal(t, "prev-token", q.Get("syncToken"))
	require.Equal(t, "true", q.Get("showDeleted"), "deletions only arrive when this is set")
	require.Equal(t, "false", q.Get("singleEvents"), "series are stored as masters, not expansions")
	require.Equal(t, "2500", q.Get("maxResults"))
	// Google refuses these next to a syncToken, so they must never be sent.
	for _, forbidden := range []string{"timeMin", "timeMax", "updatedMin", "q", "orderBy"} {
		require.Empty(t, q.Get(forbidden), "%s cannot be used with a sync token", forbidden)
	}
	require.NotContains(t, got.URL.RawQuery, "pageToken=", "an empty page token is not sent as an empty param")
}

func TestClient_ListEvents_MapsGoneAndPreconditionFailed(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":410,"message":"Sync token is no longer valid",
			"errors":[{"reason":"fullSyncRequired"}]}}`))
	}))
	_, err := c.ListEvents(context.Background(), "access", "cal", google.ListEventsParams{SyncToken: "old"})
	require.Error(t, err)
	require.True(t, google.IsGone(err), "a spent sync token has to be recognisable")
	require.False(t, google.IsRateLimited(err))
	require.Contains(t, err.Error(), "fullSyncRequired")
}

func TestClient_PatchEvent_SendsIfMatchAndReportsConflicts(t *testing.T) {
	var gotIfMatch string
	var gotBody map[string]any
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfMatch = r.Header.Get("If-Match")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if gotIfMatch == `"stale"` {
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"error":{"code":412,"errors":[{"reason":"conditionNotMet"}]}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "a", "etag": `"new"`, "summary": "Patched"})
	}))

	ev, err := c.PatchEvent(context.Background(), "access", "cal", "a", `"current"`,
		map[string]any{"summary": "Patched"}, "none")
	require.NoError(t, err)
	require.Equal(t, "Patched", ev.Summary)
	require.Equal(t, `"current"`, gotIfMatch)
	require.Equal(t, "Patched", gotBody["summary"])
	require.Len(t, gotBody, 1, "a patch sends only the fields it means to change")

	_, err = c.PatchEvent(context.Background(), "access", "cal", "a", `"stale"`,
		map[string]any{"summary": "x"}, "none")
	require.True(t, google.IsPreconditionFailed(err))
}

func TestClient_RetriesRateLimitsAndServerErrors(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":429,"errors":[{"reason":"rateLimitExceeded"}]}}`))
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":500}}`))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "nextSyncToken": "tok"})
		}
	}))

	page, err := c.ListEvents(context.Background(), "access", "cal", google.ListEventsParams{})
	require.NoError(t, err, "a rate limit and a 5xx are worth retrying, per google's own guidance")
	require.Equal(t, "tok", page.NextSyncToken)
	require.EqualValues(t, 3, calls.Load())
}

func TestClient_DoesNotRetryAnAnswer(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":410,"errors":[{"reason":"fullSyncRequired"}]}}`))
	}))
	_, err := c.ListEvents(context.Background(), "access", "cal", google.ListEventsParams{SyncToken: "x"})
	require.True(t, google.IsGone(err))
	require.EqualValues(t, 1, calls.Load(), "410 is an instruction, not a failure to retry")
}

func TestClient_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	_, err := c.ListEvents(context.Background(), "access", "cal", google.ListEventsParams{})
	require.Error(t, err)
	require.EqualValues(t, 3, calls.Load())
	require.Contains(t, err.Error(), "giving up after 3 attempts")
}

func TestClient_ForbiddenIsNotARateLimit(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"errors":[{"reason":"forbiddenForNonOrganizer"}]}}`))
	}))
	_, err := c.InsertEvent(context.Background(), "access", "cal", map[string]any{}, "none")
	require.True(t, google.IsForbidden(err))
	require.False(t, google.IsRateLimited(err), "a read-only calendar is not a quota problem")
}

func TestClient_ListCalendars_FollowsPages(t *testing.T) {
	var pages atomic.Int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/users/me/calendarList")
		if pages.Add(1) == 1 {
			require.Empty(t, r.URL.Query().Get("pageToken"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items":         []map[string]any{{"id": "a", "accessRole": "owner"}},
				"nextPageToken": "p2",
			})
			return
		}
		require.Equal(t, "p2", r.URL.Query().Get("pageToken"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "b", "accessRole": "reader", "summaryOverride": "Renamed"}},
		})
	}))

	entries, err := c.ListCalendars(context.Background(), "access")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.True(t, entries[0].Writable())
	require.False(t, entries[1].Writable())
	require.Equal(t, "Renamed", entries[1].Name(), "a renamed subscription shows the local name")
}

func TestClient_Watch_ParsesTheExpiryAndSendsTheToken(t *testing.T) {
	expiry := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Millisecond)
	var body map[string]any
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/events/watch")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": body["id"], "resourceId": "res-1",
			// Google sends a millisecond epoch, as a string.
			"expiration": strconv.FormatInt(expiry.UnixMilli(), 10),
		})
	}))

	ch, err := c.Watch(context.Background(), "access", "cal", google.WatchRequest{
		ID: "chan-1", Token: "secret-token", Address: "https://api.example/hook", TTL: 7 * 24 * time.Hour,
	})
	require.NoError(t, err)
	require.Equal(t, "chan-1", ch.ID)
	require.Equal(t, "res-1", ch.ResourceID)
	require.WithinDuration(t, expiry, ch.Expiration, time.Second)

	require.Equal(t, "web_hook", body["type"])
	require.Equal(t, "https://api.example/hook", body["address"])
	require.Equal(t, "secret-token", body["token"], "the token is the webhook's only credential")
	params, ok := body["params"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "604800", params["ttl"])
}

func TestClient_StopChannel_TreatsAnAlreadyGoneChannelAsDone(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404}}`))
	}))
	require.NoError(t, c.StopChannel(context.Background(), "access", "chan", "res"),
		"tidying up after a channel google already dropped is not an error")
}

func TestClient_RefreshToken_RecognisesADeadGrant(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	}))
	_, err := c.RefreshToken(context.Background(), "refresh-token")
	require.Error(t, err)
	require.True(t, google.IsInvalidGrant(err),
		"this is the one failure a person has to fix, so it must be distinguishable")
}

func TestClient_RefreshToken_Succeeds(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		require.Equal(t, "the-refresh-token", r.Form.Get("refresh_token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh","expires_in":3600,"token_type":"Bearer"}`))
	}))
	tok, err := c.RefreshToken(context.Background(), "the-refresh-token")
	require.NoError(t, err)
	require.Equal(t, "fresh", tok.AccessToken)
	require.WithinDuration(t, time.Now().Add(time.Hour), tok.Expiry, time.Minute)
	// Google's refresh response carries no refresh token; x/oauth2 copies the one it was given
	// forward. Either way the stored grant is never overwritten from a refresh.
	require.Equal(t, "the-refresh-token", tok.RefreshToken)
}

func TestClient_UserEmail(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer access", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"email":"me@example.com","email_verified":true}`))
	}))
	email, err := c.UserEmail(context.Background(), "access")
	require.NoError(t, err)
	require.Equal(t, "me@example.com", email)
}

func TestClient_UserEmail_EmptyIsAnError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	_, err := c.UserEmail(context.Background(), "access")
	require.ErrorContains(t, err, "no email")
}

func TestClient_RevokeToken_AcceptsAnAlreadyDeadToken(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	require.NoError(t, c.RevokeToken(context.Background(), "already-revoked"),
		"nothing left to revoke is the outcome we wanted")
}

func TestClient_MoveEvent_PutsTheDestinationInTheQuery(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "a"})
	}))
	_, err := c.MoveEvent(context.Background(), "access", "src", "a", "dst", "all")
	require.NoError(t, err)
	require.Contains(t, got.URL.Path, "/events/a/move")
	require.Equal(t, "dst", got.URL.Query().Get("destination"))
	require.Equal(t, "all", got.URL.Query().Get("sendUpdates"))
}

func TestClient_ListInstances_AsksForOneOccurrence(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "series_20260819T070000Z", "etag": `"e"`}},
		})
	}))
	items, err := c.ListInstances(context.Background(), "access", "cal", "series", "2026-08-19T07:00:00Z")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Contains(t, got.URL.Path, "/events/series/instances")
	require.Equal(t, "2026-08-19T07:00:00Z", got.URL.Query().Get("originalStart"))
}

func TestClient_EscapesCalendarIdsWithSpecialCharacters(t *testing.T) {
	var path string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	// Holiday calendars carry a '#', which would truncate an unescaped path.
	_, err := c.ListEvents(context.Background(), "access",
		"es.spain#holiday@group.v.calendar.google.com", google.ListEventsParams{})
	require.NoError(t, err)
	require.Contains(t, path, "%23holiday")
}
