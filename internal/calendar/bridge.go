// Package calendar implements the apple-calendar CLI on top of a Bridge to
// Apple Calendar (EventKit via JXA in production, a fake in tests).
package calendar

import (
	"context"
	_ "embed"
	"strconv"
	"strings"
	"time"

	"github.com/Bam679/apple-calendar-cli/internal/agentcli"
	"github.com/Bam679/apple-calendar-cli/internal/osa"
)

// Calendar is an EventKit event calendar.
type Calendar struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Account  string `json:"account"`
	Writable bool   `json:"writable"`
	Color    string `json:"color"`
}

// Event is one occurrence of an event. ID addresses exactly this occurrence
// (see MakeID and docs/adr/0005-event-ids.md).
type Event struct {
	ID         string
	Title      string
	Calendar   string
	CalendarID string
	Start      time.Time
	End        time.Time
	AllDay     bool
	Location   string
	Notes      string
	URL        string
	Recurring  bool
	Status     string
	Organizer  string
	Attendees  int
}

// Query selects events overlapping [From, To). Text, when set, lets the
// bridge pre-filter on title/location/notes; callers still filter exactly.
type Query struct {
	CalendarIDs []string
	From, To    time.Time
	Text        string
}

// NewEvent is the input for Add. For all-day events End is the last day.
type NewEvent struct {
	CalendarID  string
	Title       string
	Start, End  time.Time
	AllDay      bool
	Location    string
	Notes       string
	URL         string
	Repeat      string // "" or daily|weekly|monthly|yearly
	RepeatCount int    // 0 = no end
}

// Patch changes selected fields; nil/zero means unchanged.
type Patch struct {
	Title      *string
	Location   *string
	Notes      *string
	URL        *string
	CalendarID string
	Start, End time.Time
}

// Span selects which occurrences of a recurring event a write affects.
type Span string

// Spans.
const (
	SpanThis   Span = "this"
	SpanFuture Span = "future"
)

// Bridge is the OS boundary. Implementations must not interpret input as code.
type Bridge interface {
	Calendars(ctx context.Context) ([]Calendar, error)
	Events(ctx context.Context, q Query) ([]Event, error)
	Get(ctx context.Context, id string) (Event, error)
	Add(ctx context.Context, e NewEvent) (Event, error)
	Update(ctx context.Context, id string, p Patch, span Span) (Event, error)
	Delete(ctx context.Context, id string, span Span) error
	CreateCalendar(ctx context.Context, title string) (Calendar, error)
	// DeleteCalendar exists only for integration-test cleanup; the CLI has no
	// command for it. The JXA implementation refuses non-sandbox calendars.
	DeleteCalendar(ctx context.Context, id string, force bool) error
}

// SandboxPrefix is the only calendar title prefix DeleteCalendar accepts.
const SandboxPrefix = "agent-cli-test-"

// MakeID builds the occurrence id "<eventIdentifier>@<occurrence unix ms>".
func MakeID(eventID string, occurrenceMS int64) string {
	return eventID + "@" + strconv.FormatInt(occurrenceMS, 10)
}

// SplitID is the inverse of MakeID. An id without a numeric suffix is a bare
// eventIdentifier (enough for non-recurring events); occ is then nil.
func SplitID(id string) (eventID string, occ *int64) {
	i := strings.LastIndexByte(id, '@')
	if i <= 0 {
		return id, nil
	}
	n, err := strconv.ParseInt(id[i+1:], 10, 64)
	if err != nil {
		return id, nil
	}
	return id[:i], &n
}

//go:embed calendar.js
var script string

// JXA is the production Bridge backed by EventKit through osascript.
type JXA struct {
	Runner osa.Runner
}

// NewJXA returns the production bridge.
func NewJXA() *JXA { return &JXA{Runner: osa.Exec{}} }

type wireEvent struct {
	EventID      string `json:"event_id"`
	OccurrenceMS *int64 `json:"occurrence_ms"`
	Title        string `json:"title"`
	Calendar     string `json:"calendar"`
	CalendarID   string `json:"calendar_id"`
	StartMS      *int64 `json:"start_ms"`
	EndMS        *int64 `json:"end_ms"`
	AllDay       bool   `json:"all_day"`
	Location     string `json:"location"`
	Notes        string `json:"notes"`
	URL          string `json:"url"`
	Recurring    bool   `json:"recurring"`
	Status       string `json:"status"`
	Organizer    string `json:"organizer"`
	Attendees    int    `json:"attendees"`
}

func fromMS(v *int64) time.Time {
	if v == nil {
		return time.Time{}
	}
	return time.UnixMilli(*v).Local()
}

func toMS(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	v := t.UnixMilli()
	return &v
}

func (w wireEvent) event() Event {
	occ := w.OccurrenceMS
	if occ == nil {
		occ = w.StartMS
	}
	id := w.EventID
	if occ != nil {
		id = MakeID(w.EventID, *occ)
	}
	return Event{
		ID: id, Title: w.Title, Calendar: w.Calendar, CalendarID: w.CalendarID,
		Start: fromMS(w.StartMS), End: fromMS(w.EndMS), AllDay: w.AllDay, Location: w.Location,
		Notes: w.Notes, URL: w.URL, Recurring: w.Recurring, Status: w.Status,
		Organizer: w.Organizer, Attendees: w.Attendees,
	}
}

func idArgs(id string) map[string]any {
	eid, occ := SplitID(id)
	return map[string]any{"event_id": eid, "occurrence_ms": occ}
}

func (j *JXA) one(ctx context.Context, op string, args any) (Event, error) {
	var w wireEvent
	if err := j.Runner.Run(ctx, script, op, args, &w); err != nil {
		return Event{}, err
	}
	return w.event(), nil
}

// Calendars implements Bridge.
func (j *JXA) Calendars(ctx context.Context) ([]Calendar, error) {
	var out []Calendar
	err := j.Runner.Run(ctx, script, "calendars", nil, &out)
	return out, err
}

// Events implements Bridge.
func (j *JXA) Events(ctx context.Context, q Query) ([]Event, error) {
	var ws []wireEvent
	args := map[string]any{"calendar_ids": q.CalendarIDs, "from_ms": toMS(q.From), "to_ms": toMS(q.To), "query": q.Text}
	if err := j.Runner.Run(ctx, script, "events", args, &ws); err != nil {
		return nil, err
	}
	out := make([]Event, len(ws))
	for i, w := range ws {
		out[i] = w.event()
	}
	return out, nil
}

// Get implements Bridge.
func (j *JXA) Get(ctx context.Context, id string) (Event, error) {
	return j.one(ctx, "get", idArgs(id))
}

// Add implements Bridge.
func (j *JXA) Add(ctx context.Context, e NewEvent) (Event, error) {
	return j.one(ctx, "add", map[string]any{
		"calendar_id": e.CalendarID, "title": e.Title, "start_ms": toMS(e.Start), "end_ms": toMS(e.End),
		"all_day": e.AllDay, "location": e.Location, "notes": e.Notes, "url": e.URL,
		"repeat": e.Repeat, "repeat_count": e.RepeatCount,
	})
}

// Update implements Bridge.
func (j *JXA) Update(ctx context.Context, id string, p Patch, span Span) (Event, error) {
	args := idArgs(id)
	args["title"], args["location"], args["notes"], args["url"] = p.Title, p.Location, p.Notes, p.URL
	args["calendar_id"], args["start_ms"], args["end_ms"], args["span"] = p.CalendarID, toMS(p.Start), toMS(p.End), span
	return j.one(ctx, "update", args)
}

// Delete implements Bridge.
func (j *JXA) Delete(ctx context.Context, id string, span Span) error {
	args := idArgs(id)
	args["span"] = span
	return j.Runner.Run(ctx, script, "delete", args, nil)
}

// CreateCalendar implements Bridge.
func (j *JXA) CreateCalendar(ctx context.Context, title string) (Calendar, error) {
	var c Calendar
	err := j.Runner.Run(ctx, script, "calendar_create", map[string]string{"title": title}, &c)
	return c, err
}

// DeleteCalendar implements Bridge. It only deletes calendars whose title
// starts with SandboxPrefix, as a last line of defence for test cleanup.
func (j *JXA) DeleteCalendar(ctx context.Context, id string, force bool) error {
	cs, err := j.Calendars(ctx)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if c.ID != id {
			continue
		}
		if !strings.HasPrefix(c.Title, SandboxPrefix) {
			return agentcli.Deniedf("refusing to delete non-sandbox calendar %q", c.Title)
		}
		return j.Runner.Run(ctx, script, "calendar_delete", map[string]any{"id": id, "force": force}, nil)
	}
	return agentcli.NotFoundf("no calendar with id %s", id)
}
