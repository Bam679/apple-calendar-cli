package calendar

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Bam679/apple-calendar-cli/internal/agentcli"
)

var (
	calendarFields = []string{"id", "title", "account", "writable", "color"}
	calendarCols   = []string{"id", "title", "account"}
	eventFields    = []string{"id", "title", "calendar", "calendar_id", "start", "end", "all_day", "location",
		"notes", "url", "recurring", "status", "organizer", "attendees"}
	eventCols = []string{"id", "start", "end", "title", "calendar"}
)

const (
	maxEventsDays = 366
	// EventKit silently truncates predicates longer than four years.
	maxSearchDays = 4 * 365
	day           = 24 * time.Hour
)

// NewApp wires the CLI commands to bridge b.
func NewApp(b Bridge, version string) *agentcli.App {
	h := &handlers{b: b}
	calFlag := agentcli.Flag{Name: "calendar", Type: agentcli.Strings, Usage: "calendar name or id (repeatable)"}
	spanFlag := agentcli.Flag{Name: "span", Type: agentcli.String, Default: "this", Usage: "recurring events: this occurrence or this and all future ones", Enum: []string{"this", "future"}}
	return &agentcli.App{
		Name:         "apple-calendar",
		Version:      version,
		Summary:      "Apple Calendar for AI agents (read-only unless the policy allows writes)",
		ScopeHelp:    "calendar title, e.g. \"Work\"",
		ScopeExample: "Agent*",
		Commands: []*agentcli.Command{
			{Name: "calendars", Summary: "list calendars", Kind: agentcli.Read,
				Fields: calendarCols, AllFields: calendarFields, Run: h.calendars,
				Example: "apple-calendar calendars --json"},
			{Name: "events", Summary: "list events overlapping a range (default: today 00:00 to +7d, max 366 days)", Kind: agentcli.Read,
				Flags: []agentcli.Flag{calFlag,
					{Name: "from", Type: agentcli.String, Usage: "range start (default today 00:00)"},
					{Name: "to", Type: agentcli.String, Usage: "range end, exclusive (default --from +7d)"}},
				Fields: eventCols, AllFields: eventFields, Run: h.events,
				Example: "apple-calendar events --calendar Work --from tomorrow --to +3d"},
			{Name: "search", Summary: "find events whose title, location or notes contain text (default range: -30d to +365d)", Kind: agentcli.Read,
				Args: []agentcli.Arg{{Name: "query", Usage: "case-insensitive text", Required: true}},
				Flags: []agentcli.Flag{calFlag,
					{Name: "from", Type: agentcli.String, Usage: "range start (default -30d)"},
					{Name: "to", Type: agentcli.String, Usage: "range end, exclusive (default +365d)"}},
				Fields: eventCols, AllFields: eventFields, Run: h.search,
				Example: "apple-calendar search dentist --json"},
			{Name: "get", Summary: "show one event occurrence", Kind: agentcli.Read,
				Args:   []agentcli.Arg{{Name: "id", Usage: "event id (<eventIdentifier>@<ms>)", Required: true}},
				Fields: eventFields, AllFields: eventFields, Run: h.get},
			{Name: "doctor", Summary: "check permissions and connectivity", Kind: agentcli.Read,
				Fields: []string{"check", "ok", "detail"}, Run: h.doctor},
			{Name: "add", Summary: "create events", Kind: agentcli.Write, Op: "add",
				Args: []agentcli.Arg{{Name: "input", Usage: "'-' to read JSON/NDJSON objects from stdin"}},
				Flags: []agentcli.Flag{
					{Name: "calendar", Type: agentcli.String, Usage: "target calendar name or id"},
					{Name: "title", Type: agentcli.String, Usage: "title"},
					{Name: "start", Type: agentcli.String, Usage: "start (YYYY-MM-DD = all day) or date time"},
					{Name: "end", Type: agentcli.String, Usage: "end (all-day: last day, inclusive)"},
					{Name: "duration", Type: agentcli.String, Usage: "instead of --end: 30m, 1h30m, 2d"},
					{Name: "all-day", Type: agentcli.Bool, Usage: "all-day event"},
					{Name: "location", Type: agentcli.String, Usage: "location"},
					{Name: "notes", Type: agentcli.String, Usage: "notes"},
					{Name: "url", Type: agentcli.String, Usage: "http(s) URL"},
					{Name: "repeat", Type: agentcli.String, Usage: "recurrence", Enum: []string{"daily", "weekly", "monthly", "yearly"}},
					{Name: "repeat-count", Type: agentcli.Int, Usage: "number of occurrences (default: no end)"}},
				Stdin:  `{"calendar":"Agent","title":"…","start":"2026-12-01T09:00","duration":"30m","end":"…","all_day":false,"location":"…","notes":"…","url":"https://…","repeat":"weekly","repeat_count":4} (one per line)`,
				Fields: eventCols, AllFields: eventFields, Plan: h.add,
				Example: "apple-calendar add --calendar Agent --title 'Call Bob' --start 2026-12-01T09:00 --duration 30m --dry-run"},
			{Name: "update", Summary: "change an event (recurring: --span this|future)", Kind: agentcli.Write, Op: "update",
				Args: []agentcli.Arg{{Name: "id", Usage: "event id", Required: true}},
				Flags: []agentcli.Flag{
					{Name: "title", Type: agentcli.String, Usage: "new title"},
					{Name: "start", Type: agentcli.String, Usage: "new start (keeps the duration unless --end/--duration)"},
					{Name: "end", Type: agentcli.String, Usage: "new end"},
					{Name: "duration", Type: agentcli.String, Usage: "new duration: 30m, 1h, 2d"},
					{Name: "location", Type: agentcli.String, Usage: "new location"},
					{Name: "notes", Type: agentcli.String, Usage: "new notes"},
					{Name: "url", Type: agentcli.String, Usage: "new http(s) URL ('' clears)"},
					{Name: "calendar", Type: agentcli.String, Usage: "move to this calendar (name or id)"},
					spanFlag},
				Fields: eventCols, AllFields: eventFields, Plan: h.update},
			{Name: "delete", Summary: "delete event occurrences (recurring: --span this|future)", Kind: agentcli.Write, Op: "delete",
				Args:   []agentcli.Arg{{Name: "id", Usage: "event id(s)", Required: true, Variadic: true}},
				Flags:  []agentcli.Flag{spanFlag},
				Fields: eventCols, AllFields: eventFields, Plan: h.delete},
			{Name: "calendar-create", Summary: "create a calendar in the default account", Kind: agentcli.Write, Op: "calendar-create",
				Args:   []agentcli.Arg{{Name: "title", Usage: "new calendar title", Required: true}},
				Fields: calendarCols, AllFields: calendarFields, Plan: h.calendarCreate},
		},
	}
}

type handlers struct{ b Bridge }

func calendarItem(c Calendar) agentcli.Item {
	return agentcli.Item{"id": c.ID, "title": c.Title, "account": c.Account, "writable": c.Writable, "color": c.Color}
}

func eventItem(e Event) agentcli.Item {
	return agentcli.Item{
		"id": e.ID, "title": e.Title, "calendar": e.Calendar, "calendar_id": e.CalendarID,
		"start": agentcli.FormatTime(e.Start, e.AllDay), "end": agentcli.FormatTime(e.End, e.AllDay),
		"all_day": e.AllDay, "location": e.Location, "notes": e.Notes, "url": e.URL, "recurring": e.Recurring,
		"status": e.Status, "organizer": e.Organizer, "attendees": e.Attendees,
	}
}

func eventItems(es []Event) []agentcli.Item {
	out := make([]agentcli.Item, 0, len(es))
	for _, e := range es {
		out = append(out, eventItem(e))
	}
	return out
}

func (h *handlers) resolveCalendar(ctx context.Context, nameOrID string) (Calendar, error) {
	cs, err := h.b.Calendars(ctx)
	if err != nil {
		return Calendar{}, err
	}
	var matches []Calendar
	for _, c := range cs {
		if c.ID == nameOrID {
			return c, nil
		}
		if c.Title == nameOrID {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return Calendar{}, agentcli.NotFoundf("no calendar %q", nameOrID)
	case 1:
		return matches[0], nil
	}
	return Calendar{}, agentcli.Conflictf("%d calendars are named %q; pass the calendar id instead", len(matches), nameOrID)
}

func (h *handlers) calendars(c *agentcli.Ctx) error {
	cs, err := h.b.Calendars(c)
	if err != nil {
		return err
	}
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].Title < cs[j].Title || (cs[i].Title == cs[j].Title && cs[i].ID < cs[j].ID)
	})
	var items []agentcli.Item
	for _, cal := range cs {
		if c.CanRead(cal.Title) {
			items = append(items, calendarItem(cal))
		}
	}
	return c.List(items)
}

func midnight(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func parseWhen(c *agentcli.Ctx, s string) (time.Time, bool, error) {
	return agentcli.ParseTime(s, c.Now(), time.Local)
}

// parseDuration accepts Go durations (30m, 1h30m) and whole days (2d).
func parseDuration(s string) (time.Duration, error) {
	var d time.Duration
	var err error
	if n, ok := strings.CutSuffix(s, "d"); ok {
		var days int
		days, err = strconv.Atoi(n)
		d = time.Duration(days) * day
	} else {
		d, err = time.ParseDuration(s)
	}
	if err != nil || d <= 0 || d > maxEventsDays*day {
		return 0, agentcli.Usagef("invalid duration %q (use 30m, 1h30m or 2d)", s)
	}
	return d, nil
}

// window reads --from/--to. defTo receives the resolved start.
func window(c *agentcli.Ctx, defFrom time.Time, defTo func(time.Time) time.Time, maxDays int) (time.Time, time.Time, error) {
	from, to := defFrom, time.Time{}
	var err error
	if s := c.String("from"); s != "" {
		if from, _, err = parseWhen(c, s); err != nil {
			return from, to, err
		}
	}
	to = defTo(from)
	if s := c.String("to"); s != "" {
		if to, _, err = parseWhen(c, s); err != nil {
			return from, to, err
		}
	}
	if !to.After(from) {
		return from, to, agentcli.Usagef("--to must be after --from")
	}
	if to.After(from.AddDate(0, 0, maxDays)) {
		return from, to, agentcli.Usagef("range longer than %d days; narrow --from/--to", maxDays)
	}
	return from, to, nil
}

// fetch loads events in [from,to) for the --calendar flags, filtered by read
// scope and (if text is set) by title/location/notes, sorted by start.
func (h *handlers) fetch(c *agentcli.Ctx, from, to time.Time, text string) ([]Event, error) {
	q := Query{From: from, To: to, Text: text}
	for _, name := range c.Strings("calendar") {
		cal, err := h.resolveCalendar(c, name)
		if err != nil {
			return nil, err
		}
		if err := c.CheckRead(cal.Title); err != nil {
			return nil, err
		}
		q.CalendarIDs = append(q.CalendarIDs, cal.ID)
	}
	es, err := h.b.Events(c, q)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(text)
	out := es[:0]
	for _, e := range es {
		if !c.CanRead(e.Calendar) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(e.Title), needle) &&
			!strings.Contains(strings.ToLower(e.Location), needle) && !strings.Contains(strings.ToLower(e.Notes), needle) {
			continue
		}
		out = append(out, e)
	}
	sortEvents(out)
	return out, nil
}

func sortEvents(es []Event) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		if !a.End.Equal(b.End) {
			return a.End.Before(b.End)
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
}

func rangeKey(from, to time.Time) string {
	return from.Format(time.RFC3339) + "/" + to.Format(time.RFC3339)
}

func (h *handlers) events(c *agentcli.Ctx) error {
	from, to, err := window(c, midnight(c.Now()), func(f time.Time) time.Time { return f.AddDate(0, 0, 7) }, maxEventsDays)
	if err != nil {
		return err
	}
	es, err := h.fetch(c, from, to, "")
	if err != nil {
		return err
	}
	return c.List(eventItems(es), strings.Join(c.Strings("calendar"), "\x00"), rangeKey(from, to))
}

func (h *handlers) search(c *agentcli.Ctx) error {
	text := strings.TrimSpace(c.Args[0])
	if text == "" {
		return agentcli.Usagef("search text must not be empty")
	}
	today := midnight(c.Now())
	from, to, err := window(c, today.AddDate(0, 0, -30), func(f time.Time) time.Time {
		if d := today.AddDate(0, 0, 365); d.After(f) {
			return d
		}
		return f.AddDate(0, 0, 365)
	}, maxSearchDays)
	if err != nil {
		return err
	}
	es, err := h.fetch(c, from, to, text)
	if err != nil {
		return err
	}
	return c.List(eventItems(es), strings.ToLower(text), strings.Join(c.Strings("calendar"), "\x00"), rangeKey(from, to))
}

func (h *handlers) get(c *agentcli.Ctx) error {
	e, err := h.b.Get(c, c.Args[0])
	if err != nil {
		return err
	}
	if err := c.CheckRead(e.Calendar); err != nil {
		return err
	}
	return c.Show(eventItem(e))
}

func (h *handlers) doctor(c *agentcli.Ctx) error {
	cs, err := h.b.Calendars(c)
	row := agentcli.Item{"check": "calendar_access", "ok": err == nil}
	if err != nil {
		row["detail"] = agentcli.AsError(err).Message
	} else {
		row["detail"] = strconv.Itoa(len(cs)) + " calendars visible"
	}
	if err := c.List([]agentcli.Item{row}); err != nil {
		return err
	}
	return err
}

type addInput struct {
	Calendar    string `json:"calendar"`
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Duration    string `json:"duration"`
	AllDay      bool   `json:"all_day"`
	Location    string `json:"location"`
	Notes       string `json:"notes"`
	URL         string `json:"url"`
	Repeat      string `json:"repeat"`
	RepeatCount int    `json:"repeat_count"`
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// endFrom computes the end from --end or --duration. All-day ends are the
// last day (inclusive), so a 1d duration ends on the start day.
func endFrom(c *agentcli.Ctx, start time.Time, allDay bool, end, duration string) (time.Time, error) {
	if end != "" && duration != "" {
		return time.Time{}, agentcli.Usagef("end and duration are mutually exclusive")
	}
	var en time.Time
	switch {
	case end != "":
		t, _, err := parseWhen(c, end)
		if err != nil {
			return en, err
		}
		en = t
	case duration != "":
		d, err := parseDuration(duration)
		if err != nil {
			return en, err
		}
		if allDay {
			if d%day != 0 {
				return en, agentcli.Usagef("all-day durations are whole days, like 2d")
			}
			en = start.AddDate(0, 0, int(d/day)-1)
		} else {
			en = start.Add(d)
		}
	case allDay:
		en = start // a one-day event
	default:
		return en, agentcli.Usagef("timed events need an end or a duration")
	}
	if allDay {
		en = midnight(en)
		if en.Before(start) {
			return en, agentcli.Usagef("end must not be before start")
		}
	} else if !en.After(start) {
		return en, agentcli.Usagef("end must be after start")
	}
	return en, nil
}

func (h *handlers) newEvent(c *agentcli.Ctx, in addInput, n int) (NewEvent, Calendar, error) {
	if strings.TrimSpace(in.Title) == "" || in.Calendar == "" || in.Start == "" {
		return NewEvent{}, Calendar{}, agentcli.Usagef("item %d: calendar, title and start are required", n)
	}
	if in.URL != "" && !validURL(in.URL) {
		return NewEvent{}, Calendar{}, agentcli.Usagef("item %d: url must be http(s)", n)
	}
	switch in.Repeat {
	case "", "daily", "weekly", "monthly", "yearly":
	default:
		return NewEvent{}, Calendar{}, agentcli.Usagef("item %d: repeat must be daily|weekly|monthly|yearly", n)
	}
	if in.RepeatCount < 0 || in.RepeatCount > 1000 || (in.RepeatCount > 0 && in.Repeat == "") {
		return NewEvent{}, Calendar{}, agentcli.Usagef("item %d: repeat_count must be 1-1000 and needs repeat", n)
	}
	start, dateOnly, err := parseWhen(c, in.Start)
	if err != nil {
		return NewEvent{}, Calendar{}, err
	}
	allDay := in.AllDay || dateOnly
	if allDay {
		start = midnight(start)
	}
	end := start
	if !allDay || in.End != "" || in.Duration != "" {
		end, err = endFrom(c, start, allDay, in.End, in.Duration)
	}
	if err != nil {
		return NewEvent{}, Calendar{}, err
	}
	cal, err := h.resolveCalendar(c, in.Calendar)
	if err != nil {
		return NewEvent{}, Calendar{}, err
	}
	return NewEvent{CalendarID: cal.ID, Title: in.Title, Start: start, End: end, AllDay: allDay,
		Location: in.Location, Notes: in.Notes, URL: in.URL, Repeat: in.Repeat, RepeatCount: in.RepeatCount}, cal, nil
}

func (h *handlers) add(c *agentcli.Ctx) (*agentcli.WritePlan, error) {
	var ins []addInput
	if len(c.Args) > 0 {
		if c.Args[0] != "-" {
			return nil, agentcli.Usagef("the only positional argument is '-' (stdin)")
		}
		var err error
		if ins, err = agentcli.DecodeInputs[addInput](c.Stdin); err != nil {
			return nil, err
		}
	} else {
		count, err := c.Int("repeat-count")
		if err != nil {
			return nil, err
		}
		ins = []addInput{{Calendar: c.String("calendar"), Title: c.String("title"), Start: c.String("start"),
			End: c.String("end"), Duration: c.String("duration"), AllDay: c.Bool("all-day"), Location: c.String("location"),
			Notes: c.String("notes"), URL: c.String("url"), Repeat: c.String("repeat"), RepeatCount: count}}
	}
	var news []NewEvent
	var scopes []string
	var targets []agentcli.Item
	for i, in := range ins {
		n, cal, err := h.newEvent(c, in, i+1)
		if err != nil {
			return nil, err
		}
		news = append(news, n)
		scopes = append(scopes, cal.Title)
		targets = append(targets, agentcli.Item{"title": n.Title, "calendar": cal.Title,
			"start": agentcli.FormatTime(n.Start, n.AllDay), "end": agentcli.FormatTime(n.End, n.AllDay),
			"all_day": n.AllDay, "recurring": n.Repeat != ""})
	}
	return &agentcli.WritePlan{Scopes: scopes, Targets: targets, Apply: func(ctx context.Context) ([]agentcli.Item, error) {
		var out []agentcli.Item
		for _, n := range news {
			e, err := h.b.Add(ctx, n)
			if err != nil {
				return nil, partial(err, len(out), len(news))
			}
			out = append(out, eventItem(e))
		}
		return out, nil
	}}, nil
}

// partial annotates an error that happened after some writes succeeded.
func partial(err error, done, total int) error {
	if done == 0 {
		return err
	}
	e := *agentcli.AsError(err)
	e.Message = strconv.Itoa(done) + " of " + strconv.Itoa(total) + " writes succeeded before: " + e.Message
	return &e
}

// patchTimes applies --start/--end/--duration to e. A new start without an
// end keeps the event's duration.
func patchTimes(c *agentcli.Ctx, e Event, p *Patch) (bool, error) {
	if !c.IsSet("start") && !c.IsSet("end") && !c.IsSet("duration") {
		return false, nil
	}
	start, end := e.Start, e.End
	if c.IsSet("start") {
		t, _, err := parseWhen(c, c.String("start"))
		if err != nil {
			return false, err
		}
		if e.AllDay {
			t = midnight(t)
		}
		start, end = t, t.Add(e.End.Sub(e.Start))
	}
	if c.IsSet("end") || c.IsSet("duration") {
		var err error
		if end, err = endFrom(c, start, e.AllDay, c.String("end"), c.String("duration")); err != nil {
			return false, err
		}
	}
	p.Start, p.End = start, end
	return true, nil
}

func (h *handlers) update(c *agentcli.Ctx) (*agentcli.WritePlan, error) {
	e, err := h.b.Get(c, c.Args[0])
	if err != nil {
		return nil, err
	}
	var p Patch
	target := eventItem(e)
	scopes := []string{e.Calendar}
	changed := false
	if c.IsSet("title") {
		v := c.String("title")
		if strings.TrimSpace(v) == "" {
			return nil, agentcli.Usagef("title must not be empty")
		}
		p.Title, target["title"], changed = &v, v, true
	}
	for name, dst := range map[string]**string{"location": &p.Location, "notes": &p.Notes, "url": &p.URL} {
		if c.IsSet(name) {
			v := c.String(name)
			if name == "url" && v != "" && !validURL(v) {
				return nil, agentcli.Usagef("url must be http(s)")
			}
			*dst, target[name], changed = &v, v, true
		}
	}
	timeChanged, err := patchTimes(c, e, &p)
	if err != nil {
		return nil, err
	}
	if timeChanged {
		target["start"], target["end"], changed = agentcli.FormatTime(p.Start, e.AllDay), agentcli.FormatTime(p.End, e.AllDay), true
	}
	if c.IsSet("calendar") {
		cal, err := h.resolveCalendar(c, c.String("calendar"))
		if err != nil {
			return nil, err
		}
		if cal.ID != e.CalendarID {
			p.CalendarID, target["calendar"], target["calendar_id"], changed = cal.ID, cal.Title, cal.ID, true
			scopes = append(scopes, cal.Title)
		}
	}
	if !changed {
		return nil, agentcli.Usagef("nothing to change; pass --title, --start, --end, --duration, --location, --notes, --url or --calendar")
	}
	span := Span(c.String("span"))
	target["span"] = string(span)
	return &agentcli.WritePlan{Scopes: scopes, Targets: []agentcli.Item{target},
		Apply: func(ctx context.Context) ([]agentcli.Item, error) {
			u, err := h.b.Update(ctx, e.ID, p, span)
			if err != nil {
				return nil, err
			}
			return []agentcli.Item{eventItem(u)}, nil
		}}, nil
}

func (h *handlers) delete(c *agentcli.Ctx) (*agentcli.WritePlan, error) {
	span := Span(c.String("span"))
	var es []Event
	var scopes []string
	var targets []agentcli.Item
	for _, id := range c.Args {
		e, err := h.b.Get(c, id)
		if err != nil {
			return nil, err
		}
		es = append(es, e)
		scopes = append(scopes, e.Calendar)
		it := eventItem(e)
		it["span"] = string(span)
		targets = append(targets, it)
	}
	return &agentcli.WritePlan{Scopes: scopes, Targets: targets, Apply: func(ctx context.Context) ([]agentcli.Item, error) {
		var out []agentcli.Item
		for _, e := range es {
			if err := h.b.Delete(ctx, e.ID, span); err != nil {
				return nil, partial(err, len(out), len(es))
			}
			it := eventItem(e)
			it["deleted"] = true
			out = append(out, it)
		}
		return out, nil
	}}, nil
}

func (h *handlers) calendarCreate(c *agentcli.Ctx) (*agentcli.WritePlan, error) {
	title := strings.TrimSpace(c.Args[0])
	if title == "" {
		return nil, agentcli.Usagef("title must not be empty")
	}
	if _, err := h.resolveCalendar(c, title); err == nil {
		return nil, agentcli.Conflictf("a calendar named %q already exists", title)
	} else if agentcli.AsError(err).Exit != agentcli.ExitNotFound {
		return nil, err
	}
	return &agentcli.WritePlan{Scopes: []string{title}, Targets: []agentcli.Item{{"title": title}},
		Apply: func(ctx context.Context) ([]agentcli.Item, error) {
			cal, err := h.b.CreateCalendar(ctx, title)
			if err != nil {
				return nil, err
			}
			return []agentcli.Item{calendarItem(cal)}, nil
		}}, nil
}
