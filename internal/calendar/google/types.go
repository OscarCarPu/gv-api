// Package google is the only place in the codebase that knows how to talk to Google.
//
// It is deliberately thin: it moves JSON over HTTP, maps Google's error shapes onto typed
// errors the caller can branch on, and knows nothing about our tables. The sync rules —
// when a token is spent, what a 410 means, which fields to write — live in the calendar
// service, because that is where they are testable without a network.
package google

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Scopes requested at consent time. calendar covers reading the calendar list and both
// reading and writing events; userinfo.email is what tells us which account was connected,
// so the user does not have to type it in and cannot mistype it.
const (
	ScopeCalendar = "https://www.googleapis.com/auth/calendar"
	ScopeEmail    = "https://www.googleapis.com/auth/userinfo.email"
)

// Token is an OAuth grant. RefreshToken is empty on a refresh response: Google only issues
// one at the first consent, which is why prompt=consent is forced on the auth URL.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scope        string
}

// CalendarListEntry is one row of an account's calendar list.
type CalendarListEntry struct {
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	SummaryOverride string `json:"summaryOverride"`
	Description     string `json:"description"`
	TimeZone        string `json:"timeZone"`
	BackgroundColor string `json:"backgroundColor"`
	ForegroundColor string `json:"foregroundColor"`
	AccessRole      string `json:"accessRole"`
	Primary         bool   `json:"primary"`
	Selected        bool   `json:"selected"`
	Deleted         bool   `json:"deleted"`
}

// Name is what to show: Google puts a renamed subscription in summaryOverride and leaves the
// owner's name in summary.
func (c CalendarListEntry) Name() string {
	if c.SummaryOverride != "" {
		return c.SummaryOverride
	}
	return c.Summary
}

// Writable reports whether events can be created or edited here. reader and freeBusyReader
// cannot, and the API refuses those writes itself instead of forwarding them to be rejected.
func (c CalendarListEntry) Writable() bool {
	return c.AccessRole == "owner" || c.AccessRole == "writer"
}

// EventDateTime is Google's start/end shape: exactly one of Date (all-day, YYYY-MM-DD) or
// DateTime (RFC3339) is set.
type EventDateTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type Attendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
	Self           bool   `json:"self,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Resource       bool   `json:"resource,omitempty"`
}

type ReminderOverride struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

type Reminders struct {
	UseDefault bool               `json:"useDefault"`
	Overrides  []ReminderOverride `json:"overrides,omitempty"`
}

type Person struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName,omitempty"`
	Self        bool   `json:"self,omitempty"`
}

type ExtendedProperties struct {
	Private map[string]string `json:"private,omitempty"`
	Shared  map[string]string `json:"shared,omitempty"`
}

// Event is the subset of Google's event resource this app mirrors. Fields Google owns and we
// never write (etag, sequence, htmlLink, ...) are read-only by convention, not by type.
type Event struct {
	ID                 string              `json:"id,omitempty"`
	Etag               string              `json:"etag,omitempty"`
	Status             string              `json:"status,omitempty"`
	HTMLLink           string              `json:"htmlLink,omitempty"`
	Created            string              `json:"created,omitempty"`
	Updated            string              `json:"updated,omitempty"`
	Summary            string              `json:"summary,omitempty"`
	Description        string              `json:"description,omitempty"`
	Location           string              `json:"location,omitempty"`
	ColorID            string              `json:"colorId,omitempty"`
	Creator            *Person             `json:"creator,omitempty"`
	Organizer          *Person             `json:"organizer,omitempty"`
	Start              *EventDateTime      `json:"start,omitempty"`
	End                *EventDateTime      `json:"end,omitempty"`
	EndTimeUnspecified bool                `json:"endTimeUnspecified,omitempty"`
	Recurrence         []string            `json:"recurrence,omitempty"`
	RecurringEventID   string              `json:"recurringEventId,omitempty"`
	OriginalStartTime  *EventDateTime      `json:"originalStartTime,omitempty"`
	Transparency       string              `json:"transparency,omitempty"`
	Visibility         string              `json:"visibility,omitempty"`
	ICalUID            string              `json:"iCalUID,omitempty"`
	Sequence           int                 `json:"sequence,omitempty"`
	Attendees          []Attendee          `json:"attendees,omitempty"`
	Reminders          *Reminders          `json:"reminders,omitempty"`
	HangoutLink        string              `json:"hangoutLink,omitempty"`
	EventType          string              `json:"eventType,omitempty"`
	ExtendedProperties *ExtendedProperties `json:"extendedProperties,omitempty"`
}

// Cancelled events arrive on every incremental sync; they are the only signal that something
// was deleted.
func (e Event) Cancelled() bool { return e.Status == "cancelled" }

// EventsPage is one page of events.list. NextSyncToken is only present on the last page.
type EventsPage struct {
	Items         []Event `json:"items"`
	NextPageToken string  `json:"nextPageToken"`
	NextSyncToken string  `json:"nextSyncToken"`
	TimeZone      string  `json:"timeZone"`
}

// ListEventsParams drives events.list.
//
// Google forbids timeMin/timeMax/updatedMin/q/orderBy/iCalUID next to a syncToken and treats
// any *other* difference from the initial full sync as undefined behaviour, so this struct
// carries only what may legally vary between the two.
type ListEventsParams struct {
	SyncToken    string
	PageToken    string
	MaxResults   int
	ShowDeleted  bool
	SingleEvents bool
}

// WatchRequest asks Google to POST to Address when a calendar changes. Token is our own
// secret: it comes back in X-Goog-Channel-Token and is the only thing that authenticates
// the webhook, which by necessity is a public endpoint.
type WatchRequest struct {
	ID      string
	Token   string
	Address string
	TTL     time.Duration
}

// WatchChannel is a live push subscription. It cannot be renewed in place: when it nears
// expiry a new one is created and the old one stopped.
type WatchChannel struct {
	ID         string
	ResourceID string
	Expiration time.Time
}

// channelResponse decodes Google's channel resource, whose expiration is a millisecond
// epoch delivered as a string.
type channelResponse struct {
	ID         string `json:"id"`
	ResourceID string `json:"resourceId"`
	Expiration string `json:"expiration"`
}

func (c channelResponse) toChannel() WatchChannel {
	ch := WatchChannel{ID: c.ID, ResourceID: c.ResourceID}
	if ms, err := strconv.ParseInt(c.Expiration, 10, 64); err == nil && ms > 0 {
		ch.Expiration = time.UnixMilli(ms).UTC()
	}
	return ch
}

// APIError is a non-2xx answer from Google, decoded far enough to branch on.
type APIError struct {
	Status  int
	Reason  string
	Message string
	Body    string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("google api: %d %s: %s", e.Status, e.Reason, e.Message)
	}
	return fmt.Sprintf("google api: %d: %s", e.Status, e.Message)
}

// parseAPIError pulls the first error reason out of Google's envelope. The body is kept for
// the log because the reason alone is often too terse to act on.
func parseAPIError(status int, body []byte) *APIError {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"errors"`
			Status string `json:"status"`
		} `json:"error"`
	}
	e := &APIError{Status: status, Body: string(body)}
	if err := json.Unmarshal(body, &env); err == nil {
		e.Message = env.Error.Message
		if len(env.Error.Errors) > 0 {
			e.Reason = env.Error.Errors[0].Reason
			if e.Message == "" {
				e.Message = env.Error.Errors[0].Message
			}
		}
		if e.Reason == "" {
			e.Reason = env.Error.Status
		}
	}
	if e.Message == "" {
		e.Message = string(body)
	}
	return e
}

func statusIs(err error, status int) bool {
	var apiErr *APIError
	if !asAPIError(err, &apiErr) {
		return false
	}
	return apiErr.Status == status
}

// IsGone reports a spent sync token (or an ACL change): the local copy of that calendar has
// to be thrown away and rebuilt.
func IsGone(err error) bool { return statusIs(err, 410) }

// IsPreconditionFailed reports that the event changed in Google since the etag we sent.
func IsPreconditionFailed(err error) bool { return statusIs(err, 412) }

// IsUnauthorized reports an access token that Google refused.
func IsUnauthorized(err error) bool { return statusIs(err, 401) }

func IsNotFound(err error) bool { return statusIs(err, 404) }

// IsRateLimited covers both shapes Google uses for "slow down".
func IsRateLimited(err error) bool {
	var apiErr *APIError
	if !asAPIError(err, &apiErr) {
		return false
	}
	if apiErr.Status == 429 {
		return true
	}
	return apiErr.Status == 403 && (apiErr.Reason == "rateLimitExceeded" ||
		apiErr.Reason == "userRateLimitExceeded" || apiErr.Reason == "quotaExceeded")
}

// IsForbidden reports a write Google will not accept (read-only calendar, derived event).
func IsForbidden(err error) bool {
	return statusIs(err, 403) && !IsRateLimited(err)
}
