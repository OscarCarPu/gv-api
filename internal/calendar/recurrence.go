package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

/*
Occurrence is one materialised slot of a series.

OriginalStart is the slot the recurrence rule produced, which is the identity of the
occurrence: an override that moves an event to another day is still "the occurrence that
belonged to Tuesday", and that is what Google matches on too. Start/End are where it actually
lands once any override is applied.
*/
type Occurrence struct {
	OriginalStart time.Time
	Start         time.Time
	End           time.Time
	Exception     *EventRecord
}

/*
expandSeries materialises the occurrences of a recurring master that touch [from, to).

Two things here are easy to get wrong and are handled deliberately:

  - Zone. The rule is evaluated in the event's own IANA zone, so a weekly 09:00 stays 09:00
    across a DST change. Expanding in UTC would slide it by an hour for half the year.
  - Overrides in and out of the window. An occurrence inside the window whose override moved
    it out must disappear, and one from outside whose override moved it in must appear. Both
    directions are covered, which a naive "expand the window and overlay" misses.

Cancelled overrides are the holes in the series and are dropped from the output; they are
never deleted from the database, because the series still refers to them.
*/
func expandSeries(master EventRecord, exceptions []EventRecord, fallbackTZ string, from, to time.Time) ([]Occurrence, error) {
	loc := resolveLocation(master.StartTZ, fallbackTZ)
	duration := master.EndsAt.Sub(master.StartsAt)
	if duration < 0 {
		duration = 0
	}

	set, err := buildRRuleSet(master, loc)
	if err != nil {
		return nil, err
	}

	// Overrides are keyed by the slot they replace. The map is also the guard against
	// emitting the same occurrence twice.
	overrides := make(map[int64]*EventRecord, len(exceptions))
	for i := range exceptions {
		ex := exceptions[i]
		if ex.OriginalStartsAt == nil {
			continue
		}
		overrides[ex.OriginalStartsAt.UTC().UnixNano()] = &exceptions[i]
	}

	// Start the scan one duration early so an occurrence that began before the window but is
	// still running inside it is not missed.
	scanFrom := from.Add(-duration - time.Second)
	var out []Occurrence
	emitted := make(map[int64]bool, 16)

	for _, start := range set.Between(scanFrom, to, true) {
		key := start.UTC().UnixNano()
		if emitted[key] {
			continue
		}
		if ex, ok := overrides[key]; ok {
			emitted[key] = true
			if ex.Status == "cancelled" {
				continue
			}
			if overlaps(ex.StartsAt, ex.EndsAt, from, to) {
				out = append(out, Occurrence{
					OriginalStart: start,
					Start:         ex.StartsAt,
					End:           ex.EndsAt,
					Exception:     ex,
				})
			}
			continue
		}
		end := instanceEnd(start, duration, master.AllDay, loc)
		if !overlaps(start, end, from, to) {
			continue
		}
		emitted[key] = true
		out = append(out, Occurrence{OriginalStart: start, Start: start, End: end})
	}

	// Overrides that were moved into the window from a slot outside it. Their original slot
	// was never produced by the scan above, so they would otherwise be invisible.
	for key, ex := range overrides {
		if emitted[key] || ex.Status == "cancelled" {
			continue
		}
		if overlaps(ex.StartsAt, ex.EndsAt, from, to) {
			original := time.Unix(0, key).UTC()
			out = append(out, Occurrence{
				OriginalStart: original,
				Start:         ex.StartsAt,
				End:           ex.EndsAt,
				Exception:     ex,
			})
			emitted[key] = true
		}
	}

	sortOccurrences(out)
	return out, nil
}

// instanceEnd advances the start by the master's length.
//
// An all-day event is advanced in whole local days rather than by an absolute duration: a
// day containing a DST change is 23 or 25 hours long, and adding 24h to it would put the end
// an hour off midnight and make the event look like it spills into the next day.
func instanceEnd(start time.Time, duration time.Duration, allDay bool, loc *time.Location) time.Time {
	if !allDay {
		return start.Add(duration)
	}
	days := int(duration.Hours() / 24)
	if days < 1 {
		days = 1
	}
	local := start.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day()+days,
		local.Hour(), local.Minute(), 0, 0, loc)
}

func overlaps(start, end, from, to time.Time) bool {
	if !start.Before(to) {
		return false
	}
	if end.Equal(start) {
		// A zero-length event still happens at an instant inside the window.
		return !start.Before(from)
	}
	return end.After(from)
}

func sortOccurrences(occ []Occurrence) {
	for i := 1; i < len(occ); i++ {
		for j := i; j > 0 && occ[j].Start.Before(occ[j-1].Start); j-- {
			occ[j], occ[j-1] = occ[j-1], occ[j]
		}
	}
}

func resolveLocation(names ...string) *time.Location {
	for _, name := range names {
		if name == "" {
			continue
		}
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.UTC
}

/*
buildRRuleSet turns Google's recurrence lines into something rrule-go can evaluate.

Google hands back the lines of an iCalendar VEVENT without the DTSTART, since the start is
carried by the event's own start field, so it has to be put back. The lines themselves are
passed through as they arrive, with one fix: an EXDATE or RDATE written as a bare date
(VALUE=DATE, which Google uses for all-day series) is expanded to midnight in the event's
zone, because the parser only accepts date-times.
*/
func buildRRuleSet(master EventRecord, loc *time.Location) (*rrule.Set, error) {
	local := master.StartsAt.In(loc)
	lines := []string{
		fmt.Sprintf("DTSTART;TZID=%s:%s", loc.String(), local.Format("20060102T150405")),
	}
	for _, raw := range master.Recurrence {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lines = append(lines, normalizeRecurrenceLine(line, loc))
	}

	set, err := rrule.StrSliceToRRuleSetInLoc(lines, loc)
	if err != nil {
		return nil, fmt.Errorf("parse recurrence %v: %w", master.Recurrence, err)
	}
	return set, nil
}

// normalizeRecurrenceLine rewrites the date-only form of EXDATE/RDATE, which Google emits for
// all-day series and rrule-go does not accept.
func normalizeRecurrenceLine(line string, loc *time.Location) string {
	name, params, values, ok := splitContentLine(line)
	if !ok || (name != "EXDATE" && name != "RDATE") {
		return line
	}
	if !strings.Contains(strings.ToUpper(params), "VALUE=DATE") {
		return line
	}
	parts := strings.Split(values, ",")
	for i, v := range parts {
		v = strings.TrimSpace(v)
		if len(v) == 8 {
			parts[i] = v + "T000000"
		} else {
			parts[i] = v
		}
	}
	return fmt.Sprintf("%s;TZID=%s:%s", name, loc.String(), strings.Join(parts, ","))
}

// splitContentLine breaks "NAME;PARAMS:VALUES" apart.
func splitContentLine(line string) (name, params, values string, ok bool) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", "", "", false
	}
	head, values := line[:colon], line[colon+1:]
	if semi := strings.Index(head, ";"); semi >= 0 {
		return head[:semi], head[semi+1:], values, true
	}
	return head, "", values, true
}
