// Package google is the only place that talks to Google. It moves JSON over HTTP and maps
// error shapes onto typed errors; the sync rules live in the calendar service.
package google

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Scopes requested at consent time. userinfo.email identifies the connected account.
const (
	ScopeCalendar = "https://www.googleapis.com/auth/calendar"
	ScopeEmail    = "https://www.googleapis.com/auth/userinfo.email"
)

// Token is an OAuth grant. RefreshToken is only issued at first consent, hence prompt=consent.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scope        string
}

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

// Name prefers summaryOverride, where Google puts a renamed subscription.
func (c CalendarListEntry) Name() string {
	if c.SummaryOverride != "" {
		return c.SummaryOverride
	}
	return c.Summary
}

// Writable reports whether events can be created or edited here.
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

// Event is the subset of Google's event resource this app mirrors.
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

// Cancelled events are the only signal of a deletion on incremental syncs.
func (e Event) Cancelled() bool { return e.Status == "cancelled" }

type EventsPage struct {
	Items         []Event `json:"items"`
	NextPageToken string  `json:"nextPageToken"`
	NextSyncToken string  `json:"nextSyncToken"`
	TimeZone      string  `json:"timeZone"`
}

// ListEventsParams drives events.list. It carries only what may differ between a full sync and
// a syncToken request, since Google forbids the rest.
type ListEventsParams struct {
	SyncToken    string
	PageToken    string
	MaxResults   int
	ShowDeleted  bool
	SingleEvents bool
}

// WatchRequest asks Google to POST to Address when a calendar changes. Token comes back in
// X-Goog-Channel-Token and authenticates the public webhook.
type WatchRequest struct {
	ID      string
	Token   string
	Address string
	TTL     time.Duration
}

// WatchChannel is a live push subscription; it cannot be renewed in place.
type WatchChannel struct {
	ID         string
	ResourceID string
	Expiration time.Time
}

// channelResponse decodes a channel whose expiration is a millisecond epoch string.
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

// parseAPIError pulls the first error reason out of Google's envelope, keeping the body for logs.
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

// IsGone reports a spent sync token (or an ACL change): the calendar must be rebuilt.
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
