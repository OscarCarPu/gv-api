package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

/*
Fake is an in-memory Google, and it is the reason the sync logic is testable at all.

It reproduces the four behaviours that shape the design and that a hand-written stub
usually skips:

  - sync tokens: a full list hands out a token, an incremental list returns only what changed
    since it, and a token from before ExpireSyncTokens returns 410 Gone.
  - etags: every write bumps one, and a write carrying a stale etag is a 412.
  - grants dying: SetInvalidGrant makes refreshes fail the way a revoked account does.
  - instances: patching an occurrence of a series that has no override yet materialises one,
    which is what Google does and what the "edit only this one" path depends on.

Everything is guarded by one mutex: tests drive it from the sync worker's goroutines.
*/
type Fake struct {
	mu sync.Mutex

	accounts    map[string]*fakeAccount // by refresh token
	byAccess    map[string]*fakeAccount // by access token
	codes       map[string]string       // auth code -> email
	tokenGen    int64                   // bumped by ExpireSyncTokens
	seq         int64                   // event version counter, shared by all calendars
	nextID      int64
	channels    map[string]WatchChannel
	WatchTTL    time.Duration
	PageSize    int
	AuthURLBase string

	// FailWith, when set, is returned by the next Calendar API call and then cleared. It is
	// how a test injects a 403 or a transport error at an exact point in a sync.
	FailWith error
	// Calls records every Calendar API method invoked, in order, for tests that care about
	// how many round-trips a path costs.
	Calls []string
}

type fakeAccount struct {
	email        string
	refreshToken string
	accessToken  string
	expiry       time.Time
	invalidGrant bool
	revoked      bool
	calendars    []CalendarListEntry
	events       map[string]map[string]*fakeEvent // calendar id -> event id -> event
}

type fakeEvent struct {
	ev      Event
	version int64
}

func NewFake() *Fake {
	return &Fake{
		accounts:    map[string]*fakeAccount{},
		byAccess:    map[string]*fakeAccount{},
		codes:       map[string]string{},
		channels:    map[string]WatchChannel{},
		WatchTTL:    7 * 24 * time.Hour,
		PageSize:    2500,
		AuthURLBase: "https://accounts.example/o/oauth2/auth",
		nextID:      1,
	}
}

// --- test controls -------------------------------------------------------------------

// AddAccount registers an account with the given calendars and returns the auth code that
// the OAuth callback would arrive with.
func (f *Fake) AddAccount(email string, calendars ...CalendarListEntry) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc := &fakeAccount{
		email:        email,
		refreshToken: "refresh-" + email,
		accessToken:  "access-" + email,
		expiry:       time.Now().Add(time.Hour),
		calendars:    calendars,
		events:       map[string]map[string]*fakeEvent{},
	}
	for _, c := range calendars {
		acc.events[c.ID] = map[string]*fakeEvent{}
	}
	f.accounts[acc.refreshToken] = acc
	f.byAccess[acc.accessToken] = acc
	code := "code-" + email
	f.codes[code] = email
	return code
}

// AccessToken is the token a test hands to the client methods directly.
func (f *Fake) AccessToken(email string) string { return "access-" + email }

// PutEvent inserts or replaces an event as if it had been created in Google's UI.
func (f *Fake) PutEvent(email, calendarID string, ev Event) Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc := f.accountByEmail(email)
	if acc == nil {
		panic("fake: unknown account " + email)
	}
	if ev.ID == "" {
		ev.ID = f.newID()
	}
	if ev.Status == "" {
		ev.Status = "confirmed"
	}
	if ev.ICalUID == "" {
		ev.ICalUID = ev.ID + "@google.com"
	}
	stored := f.store(acc, calendarID, ev)
	return stored.ev
}

// DeleteEventDirect cancels an event the way a delete in Google's UI would: the row stays,
// marked cancelled, so the next incremental sync sees it.
func (f *Fake) DeleteEventDirect(email, calendarID, eventID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc := f.accountByEmail(email)
	if acc == nil {
		return
	}
	if e, ok := acc.events[calendarID][eventID]; ok {
		e.ev.Status = "cancelled"
		f.seq++
		e.version = f.seq
		e.ev.Etag = f.etag()
	}
}

// ExpireSyncTokens makes every token handed out so far answer 410 Gone.
func (f *Fake) ExpireSyncTokens() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenGen++
}

// SetInvalidGrant makes this account's refreshes fail like a revoked grant.
func (f *Fake) SetInvalidGrant(email string, invalid bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if acc := f.accountByEmail(email); acc != nil {
		acc.invalidGrant = invalid
	}
}

// Revoked reports whether RevokeToken was called for the account.
func (f *Fake) Revoked(email string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc := f.accountByEmail(email)
	return acc != nil && acc.revoked
}

// Event returns the stored event, for asserting what a write actually did.
func (f *Fake) Event(email, calendarID, eventID string) (Event, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc := f.accountByEmail(email)
	if acc == nil {
		return Event{}, false
	}
	e, ok := acc.events[calendarID][eventID]
	if !ok {
		return Event{}, false
	}
	return e.ev, true
}

// Events lists a calendar's stored events, cancelled ones included.
func (f *Fake) Events(email, calendarID string) []Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc := f.accountByEmail(email)
	if acc == nil {
		return nil
	}
	out := make([]Event, 0, len(acc.events[calendarID]))
	for _, e := range acc.events[calendarID] {
		out = append(out, e.ev)
	}
	return out
}

// Channels returns the live push channels, so a test can check they were renewed and the old
// ones stopped.
func (f *Fake) Channels() map[string]WatchChannel {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]WatchChannel, len(f.channels))
	for k, v := range f.channels {
		out[k] = v
	}
	return out
}

func (f *Fake) CallCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if c == method {
			n++
		}
	}
	return n
}

// --- helpers (mutex already held) ----------------------------------------------------

func (f *Fake) accountByEmail(email string) *fakeAccount {
	for _, a := range f.accounts {
		if a.email == email {
			return a
		}
	}
	return nil
}

func (f *Fake) newID() string {
	f.nextID++
	return fmt.Sprintf("ev%d", f.nextID)
}

func (f *Fake) etag() string { return fmt.Sprintf(`"etag-%d"`, f.seq) }

func (f *Fake) store(acc *fakeAccount, calendarID string, ev Event) *fakeEvent {
	if acc.events[calendarID] == nil {
		acc.events[calendarID] = map[string]*fakeEvent{}
	}
	f.seq++
	ev.Etag = f.etag()
	ev.Updated = time.Now().UTC().Format(time.RFC3339)
	e := &fakeEvent{ev: ev, version: f.seq}
	acc.events[calendarID][ev.ID] = e
	return e
}

func (f *Fake) record(method string) error {
	f.Calls = append(f.Calls, method)
	if f.FailWith != nil {
		err := f.FailWith
		f.FailWith = nil
		return err
	}
	return nil
}

func (f *Fake) auth(accessToken string) (*fakeAccount, error) {
	acc, ok := f.byAccess[accessToken]
	if !ok {
		return nil, &APIError{Status: 401, Reason: "authError", Message: "Invalid Credentials"}
	}
	return acc, nil
}

// --- Client implementation -----------------------------------------------------------

func (f *Fake) AuthURL(state string) string {
	return f.AuthURLBase + "?access_type=offline&prompt=consent&state=" + state
}

func (f *Fake) ExchangeCode(_ context.Context, code string) (*Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	email, ok := f.codes[code]
	if !ok {
		return nil, fmt.Errorf("fake: unknown code %q", code)
	}
	acc := f.accountByEmail(email)
	return &Token{
		AccessToken:  acc.accessToken,
		RefreshToken: acc.refreshToken,
		Expiry:       time.Now().Add(time.Hour),
		Scope:        ScopeCalendar + " " + ScopeEmail,
	}, nil
}

func (f *Fake) RefreshToken(_ context.Context, refreshToken string) (*Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, ok := f.accounts[refreshToken]
	if !ok || acc.invalidGrant {
		return nil, invalidGrantError()
	}
	// A refresh never returns a new refresh token, same as the real thing.
	return &Token{AccessToken: acc.accessToken, Expiry: time.Now().Add(time.Hour)}, nil
}

func (f *Fake) RevokeToken(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if acc, ok := f.accounts[token]; ok {
		acc.revoked = true
		acc.invalidGrant = true
		return nil
	}
	if acc, ok := f.byAccess[token]; ok {
		acc.revoked = true
		acc.invalidGrant = true
	}
	return nil
}

func (f *Fake) UserEmail(_ context.Context, accessToken string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, err := f.auth(accessToken)
	if err != nil {
		return "", err
	}
	return acc.email, nil
}

func (f *Fake) ListCalendars(_ context.Context, accessToken string) ([]CalendarListEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListCalendars"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	return append([]CalendarListEntry(nil), acc.calendars...), nil
}

/*
ListEvents implements the part that matters: full sync when no token is given, only the
changes since the token when one is, 410 when the token predates ExpireSyncTokens, and a
sync token attached to the last page only.
*/
func (f *Fake) ListEvents(_ context.Context, accessToken, calendarID string, p ListEventsParams) (*EventsPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListEvents"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}

	since := int64(0)
	if p.SyncToken != "" {
		gen, ver, ok := parseFakeToken(p.SyncToken)
		if !ok {
			return nil, &APIError{Status: 400, Reason: "invalid", Message: "invalid sync token"}
		}
		if gen != f.tokenGen {
			return nil, &APIError{Status: 410, Reason: "fullSyncRequired", Message: "Sync token is no longer valid"}
		}
		since = ver
	}

	// Deterministic order: version ascending, which is also "oldest change first".
	var picked []*fakeEvent
	for _, e := range acc.events[calendarID] {
		if e.version <= since {
			continue
		}
		// A full sync does not report events that were already cancelled before it ran;
		// an incremental one must, or the client never learns about the deletion.
		if since == 0 && e.ev.Cancelled() && !p.ShowDeleted {
			continue
		}
		picked = append(picked, e)
	}
	for i := 0; i < len(picked); i++ {
		for j := i + 1; j < len(picked); j++ {
			if picked[j].version < picked[i].version {
				picked[i], picked[j] = picked[j], picked[i]
			}
		}
	}

	offset := 0
	if p.PageToken != "" {
		offset, _ = strconv.Atoi(p.PageToken)
	}
	size := f.PageSize
	if p.MaxResults > 0 && p.MaxResults < size {
		size = p.MaxResults
	}
	end := min(offset+size, len(picked))

	page := &EventsPage{TimeZone: "Europe/Madrid"}
	high := since
	for _, e := range picked[offset:end] {
		page.Items = append(page.Items, e.ev)
		high = max(high, e.version)
	}
	if end < len(picked) {
		page.NextPageToken = strconv.Itoa(end)
		return page, nil
	}
	// Last page: the token must cover everything the caller has now seen, including changes
	// that landed after this listing started.
	page.NextSyncToken = fmt.Sprintf("tok:%d:%d", f.tokenGen, max(high, f.seq))
	return page, nil
}

func parseFakeToken(tok string) (gen, ver int64, ok bool) {
	parts := strings.Split(tok, ":")
	if len(parts) != 3 || parts[0] != "tok" {
		return 0, 0, false
	}
	g, err1 := strconv.ParseInt(parts[1], 10, 64)
	v, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return g, v, true
}

func (f *Fake) GetEvent(_ context.Context, accessToken, calendarID, eventID string) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("GetEvent"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	e, ok := acc.events[calendarID][eventID]
	if !ok {
		return nil, &APIError{Status: 404, Reason: "notFound", Message: "Not Found"}
	}
	ev := e.ev
	return &ev, nil
}

// ListInstances answers with the stored override for that occurrence when there is one, and
// otherwise materialises the instance the way Google does, so the caller gets an id it can
// patch.
func (f *Fake) ListInstances(_ context.Context, accessToken, calendarID, eventID, originalStart string) ([]Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListInstances"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	master, ok := acc.events[calendarID][eventID]
	if !ok {
		return nil, &APIError{Status: 404, Reason: "notFound", Message: "Not Found"}
	}
	for _, e := range acc.events[calendarID] {
		if e.ev.RecurringEventID == eventID && e.ev.OriginalStartTime != nil &&
			(e.ev.OriginalStartTime.DateTime == originalStart || e.ev.OriginalStartTime.Date == originalStart) {
			ev := e.ev
			return []Event{ev}, nil
		}
	}
	inst := master.ev
	inst.ID = instanceID(eventID, originalStart)
	inst.Recurrence = nil
	inst.RecurringEventID = eventID
	inst.OriginalStartTime = dateTimeFor(originalStart)
	inst.Start = dateTimeFor(originalStart)
	inst.End = nil
	return []Event{inst}, nil
}

func dateTimeFor(s string) *EventDateTime {
	if len(s) == len("2006-01-02") {
		return &EventDateTime{Date: s}
	}
	return &EventDateTime{DateTime: s}
}

func instanceID(masterID, originalStart string) string {
	t, err := time.Parse(time.RFC3339, originalStart)
	if err != nil {
		return masterID + "_" + strings.ReplaceAll(originalStart, "-", "")
	}
	return masterID + "_" + t.UTC().Format("20060102T150405Z")
}

func (f *Fake) InsertEvent(_ context.Context, accessToken, calendarID string, body map[string]any, _ string) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("InsertEvent"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	if !f.writable(acc, calendarID) {
		return nil, &APIError{Status: 403, Reason: "forbiddenForNonOrganizer", Message: "read-only calendar"}
	}
	var ev Event
	if err := remarshal(body, &ev); err != nil {
		return nil, &APIError{Status: 400, Reason: "invalid", Message: err.Error()}
	}
	ev.ID = f.newID()
	ev.Status = "confirmed"
	ev.ICalUID = ev.ID + "@google.com"
	ev.HTMLLink = "https://calendar.google.com/event?eid=" + ev.ID
	stored := f.store(acc, calendarID, ev)
	out := stored.ev
	return &out, nil
}

func (f *Fake) PatchEvent(_ context.Context, accessToken, calendarID, eventID, ifMatch string, body map[string]any, _ string) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("PatchEvent"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	if !f.writable(acc, calendarID) {
		return nil, &APIError{Status: 403, Reason: "forbiddenForNonOrganizer", Message: "read-only calendar"}
	}

	e, ok := acc.events[calendarID][eventID]
	if !ok {
		// Patching an occurrence that has no override yet creates one, which is how Google
		// turns "edit just this instance" into a real event.
		masterID, originalStart, isInstance := splitInstanceID(eventID)
		master, hasMaster := acc.events[calendarID][masterID]
		if !isInstance || !hasMaster {
			return nil, &APIError{Status: 404, Reason: "notFound", Message: "Not Found"}
		}
		inst := master.ev
		inst.ID = eventID
		inst.Recurrence = nil
		inst.RecurringEventID = masterID
		inst.OriginalStartTime = &EventDateTime{DateTime: originalStart}
		inst.Start = &EventDateTime{DateTime: originalStart}
		e = f.store(acc, calendarID, inst)
	} else if ifMatch != "" && ifMatch != e.ev.Etag {
		return nil, &APIError{Status: 412, Reason: "conditionNotMet", Message: "Precondition Failed"}
	}

	patched := e.ev
	if err := remarshal(body, &patched); err != nil {
		return nil, &APIError{Status: 400, Reason: "invalid", Message: err.Error()}
	}
	patched.ID = e.ev.ID
	// A patch that sets start without end (or the other way round) keeps the other side, and
	// clearing recurrence is expressed as an explicit empty list.
	if raw, ok := body["recurrence"]; ok {
		if list, isList := raw.([]string); isList && len(list) == 0 {
			patched.Recurrence = nil
		}
	}
	stored := f.store(acc, calendarID, patched)
	out := stored.ev
	return &out, nil
}

func splitInstanceID(id string) (masterID, originalStart string, ok bool) {
	i := strings.LastIndex(id, "_")
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	t, err := time.Parse("20060102T150405Z", id[i+1:])
	if err != nil {
		return "", "", false
	}
	return id[:i], t.UTC().Format(time.RFC3339), true
}

func (f *Fake) DeleteEvent(_ context.Context, accessToken, calendarID, eventID, ifMatch, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("DeleteEvent"); err != nil {
		return err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return err
	}
	if !f.writable(acc, calendarID) {
		return &APIError{Status: 403, Reason: "forbiddenForNonOrganizer", Message: "read-only calendar"}
	}
	e, ok := acc.events[calendarID][eventID]
	if !ok {
		masterID, originalStart, isInstance := splitInstanceID(eventID)
		master, hasMaster := acc.events[calendarID][masterID]
		if !isInstance || !hasMaster {
			return &APIError{Status: 404, Reason: "notFound", Message: "Not Found"}
		}
		// Cancelling an occurrence of a series leaves a cancelled exception behind: that row
		// is the hole in the series and it has to survive.
		inst := master.ev
		inst.ID = eventID
		inst.Recurrence = nil
		inst.Status = "cancelled"
		inst.RecurringEventID = masterID
		inst.OriginalStartTime = &EventDateTime{DateTime: originalStart}
		inst.Start = &EventDateTime{DateTime: originalStart}
		f.store(acc, calendarID, inst)
		return nil
	}
	if ifMatch != "" && ifMatch != e.ev.Etag {
		return &APIError{Status: 412, Reason: "conditionNotMet", Message: "Precondition Failed"}
	}
	e.ev.Status = "cancelled"
	f.seq++
	e.version = f.seq
	e.ev.Etag = f.etag()
	return nil
}

func (f *Fake) MoveEvent(_ context.Context, accessToken, calendarID, eventID, destination, _ string) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("MoveEvent"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	e, ok := acc.events[calendarID][eventID]
	if !ok {
		return nil, &APIError{Status: 404, Reason: "notFound", Message: "Not Found"}
	}
	if _, dstKnown := acc.events[destination]; !dstKnown && !f.hasCalendar(acc, destination) {
		return nil, &APIError{Status: 404, Reason: "notFound", Message: "destination not found"}
	}
	moved := e.ev
	delete(acc.events[calendarID], eventID)
	// The source row is gone for good here; Google reports the removal on the source
	// calendar's next incremental sync.
	f.seq++
	stored := f.store(acc, destination, moved)
	out := stored.ev
	return &out, nil
}

func (f *Fake) Watch(_ context.Context, accessToken, calendarID string, req WatchRequest) (*WatchChannel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Watch"); err != nil {
		return nil, err
	}
	acc, err := f.auth(accessToken)
	if err != nil {
		return nil, err
	}
	if !f.hasCalendar(acc, calendarID) {
		return nil, &APIError{Status: 404, Reason: "notFound", Message: "Not Found"}
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = f.WatchTTL
	}
	ch := WatchChannel{
		ID:         req.ID,
		ResourceID: "res-" + calendarID + "-" + req.ID,
		Expiration: time.Now().Add(ttl).UTC(),
	}
	f.channels[ch.ID] = ch
	return &ch, nil
}

func (f *Fake) StopChannel(_ context.Context, accessToken, channelID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("StopChannel"); err != nil {
		return err
	}
	if _, err := f.auth(accessToken); err != nil {
		return err
	}
	delete(f.channels, channelID)
	return nil
}

func (f *Fake) hasCalendar(acc *fakeAccount, id string) bool {
	for _, c := range acc.calendars {
		if c.ID == id {
			return true
		}
	}
	return false
}

func (f *Fake) writable(acc *fakeAccount, id string) bool {
	for _, c := range acc.calendars {
		if c.ID == id {
			return c.Writable()
		}
	}
	return false
}

// remarshal applies a patch body to a struct with JSON merge semantics: keys present in the
// body overwrite, keys absent leave the field alone. That is exactly what events.patch does,
// so the fake gets Google's behaviour for free instead of reimplementing it field by field.
func remarshal(body map[string]any, into any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

// invalidGrantError is the error a dead refresh token produces, in the shape
// golang.org/x/oauth2 would produce it, so IsInvalidGrant recognises it.
func invalidGrantError() error {
	return &oauth2.RetrieveError{
		Response:  &http.Response{StatusCode: http.StatusBadRequest},
		Body:      []byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`),
		ErrorCode: "invalid_grant",
	}
}
