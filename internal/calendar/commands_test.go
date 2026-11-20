package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Bam679/apple-calendar-cli/internal/agentcli"
)

var update = flag.Bool("update", false, "rewrite golden files")

// systemLocal is the real zone; integration tests restore it because
// EventKit places all-day events in the system zone.
var systemLocal = time.Local

// TestMain pins the zone so golden files match on every machine.
func TestMain(m *testing.M) {
	systemLocal = time.Local
	time.Local = time.UTC
	os.Exit(m.Run())
}

// fakeBridge is an in-memory Bridge. Fixture data is synthetic only.
type fakeBridge struct {
	cals   []Calendar
	events []Event
	next   int
	writes int
	fail   error
	last   Query
}

func at(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.Local) }

func ev(eid string, title, cal, calID string, start, end time.Time) Event {
	return Event{ID: MakeID(eid, start.UnixMilli()), Title: title, Calendar: cal, CalendarID: calID,
		Start: start, End: end, Status: "confirmed"}
}

func newFake() *fakeBridge {
	f := &fakeBridge{
		cals: []Calendar{
			{ID: "C1", Title: "Work", Account: "iCloud", Writable: true, Color: "#1badf8"},
			{ID: "C2", Title: "Agent Sandbox", Account: "iCloud", Writable: true, Color: "#63da38"},
			{ID: "C3", Title: "Private", Account: "iCloud", Writable: true},
			{ID: "C4", Title: "Dup", Account: "iCloud"}, {ID: "C5", Title: "Dup", Account: "Local"},
		},
	}
	sync := ev("E1", "Team sync", "Work", "C1", at(20, 10, 0), at(20, 10, 30))
	sync.Location, sync.Organizer, sync.Attendees = "Room 1", "Alex Example", 3
	plan := ev("E2", "Planning day", "Agent Sandbox", "C2", at(21, 0, 0), at(21, 23, 59).Add(59*time.Second))
	plan.AllDay, plan.Notes, plan.URL = true, "bring\nnotes", "https://example.com/plan"
	diary := ev("E3", "Diary", "Private", "C3", at(22, 9, 0), at(22, 10, 0))
	far := ev("E5", "Conference", "Work", "C1", time.Date(2027, 3, 1, 9, 0, 0, 0, time.Local), time.Date(2027, 3, 1, 17, 0, 0, 0, time.Local))
	far.Location = "Example Hall"
	f.events = []Event{sync, plan, diary, far}
	for d := 23; d <= 25; d++ {
		s := ev("S1", "Standup", "Agent Sandbox", "C2", at(d, 9, 0), at(d, 9, 15))
		s.Recurring = true
		f.events = append(f.events, s)
	}
	return f
}

func (f *fakeBridge) Calendars(context.Context) ([]Calendar, error) {
	return append([]Calendar{}, f.cals...), f.fail
}

func (f *fakeBridge) Events(_ context.Context, q Query) ([]Event, error) {
	f.last = q
	var out []Event
	for _, e := range f.events {
		if len(q.CalendarIDs) > 0 && !slices.Contains(q.CalendarIDs, e.CalendarID) {
			continue
		}
		if !e.End.After(q.From) && !e.Start.Equal(q.From) || !e.Start.Before(q.To) {
			continue
		}
		out = append(out, e)
	}
	return out, f.fail
}

func (f *fakeBridge) idx(id string) int {
	for i, e := range f.events {
		if e.ID == id {
			return i
		}
	}
	eid, _ := SplitID(id)
	for i, e := range f.events {
		if prefix, _ := SplitID(e.ID); prefix == eid && !e.Recurring {
			return i
		}
	}
	return -1
}

func (f *fakeBridge) Get(_ context.Context, id string) (Event, error) {
	if i := f.idx(id); i >= 0 {
		return f.events[i], nil
	}
	return Event{}, agentcli.NotFoundf("no event with id %s", id)
}

func (f *fakeBridge) calTitle(id string) string {
	for _, c := range f.cals {
		if c.ID == id {
			return c.Title
		}
	}
	return ""
}

func (f *fakeBridge) Add(_ context.Context, n NewEvent) (Event, error) {
	f.writes++
	if f.fail != nil {
		return Event{}, f.fail
	}
	f.next++
	e := ev(fmt.Sprintf("N%d", f.next), n.Title, f.calTitle(n.CalendarID), n.CalendarID, n.Start, n.End)
	e.AllDay, e.Location, e.Notes, e.URL, e.Recurring = n.AllDay, n.Location, n.Notes, n.URL, n.Repeat != ""
	f.events = append(f.events, e)
	return e, nil
}

func (f *fakeBridge) Update(_ context.Context, id string, p Patch, _ Span) (Event, error) {
	f.writes++
	e := &f.events[f.idx(id)]
	for src, dst := range map[*string]*string{p.Title: &e.Title, p.Location: &e.Location, p.Notes: &e.Notes, p.URL: &e.URL} {
		if src != nil {
			*dst = *src
		}
	}
	if p.CalendarID != "" {
		e.CalendarID, e.Calendar = p.CalendarID, f.calTitle(p.CalendarID)
	}
	if !p.Start.IsZero() {
		e.Start, e.End = p.Start, p.End
	}
	return *e, nil
}

func (f *fakeBridge) Delete(_ context.Context, id string, span Span) error {
	f.writes++
	if f.fail != nil {
		return f.fail
	}
	target := f.events[f.idx(id)]
	eid, _ := SplitID(target.ID)
	out := f.events[:0]
	for _, e := range f.events {
		prefix, _ := SplitID(e.ID)
		if e.ID == target.ID || (span == SpanFuture && target.Recurring && prefix == eid && e.Start.After(target.Start)) {
			continue
		}
		out = append(out, e)
	}
	f.events = out
	return nil
}

func (f *fakeBridge) CreateCalendar(_ context.Context, title string) (Calendar, error) {
	f.writes++
	c := Calendar{ID: "C" + title, Title: title, Account: "iCloud", Writable: true}
	f.cals = append(f.cals, c)
	return c, nil
}

func (f *fakeBridge) DeleteCalendar(_ context.Context, id string, _ bool) error {
	f.writes++
	for i, c := range f.cals {
		if c.ID == id {
			f.cals = append(f.cals[:i], f.cals[i+1:]...)
			return nil
		}
	}
	return agentcli.NotFoundf("no calendar")
}

var (
	readOnly  = agentcli.Effective{Layers: []agentcli.Policy{agentcli.DefaultPolicy()}}
	sandboxRW = agentcli.Effective{Layers: []agentcli.Policy{{Version: 1,
		Operations: agentcli.OpRule{Allow: []string{agentcli.OpRead, agentcli.OpWrite}},
		Scopes: agentcli.ScopeRules{Read: agentcli.Rule{Allow: []string{"*"}, Deny: []string{"Private"}},
			Write: agentcli.Rule{Allow: []string{"Agent*"}}}}}}
)

func run(t *testing.T, b *fakeBridge, pol agentcli.Effective, stdin string, args ...string) (int, string, string) {
	t.Helper()
	app := NewApp(b, "test")
	app.LoadPolicy = func() (agentcli.Effective, error) { return pol, nil }
	app.Now = func() time.Time { return at(20, 12, 0) }
	var out, errb bytes.Buffer
	code := app.Main(context.Background(), args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		_ = os.MkdirAll(filepath.Dir(p), 0o750)
		if err := os.WriteFile(p, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("missing golden %s: run go test -update", p)
	}
	if got != string(want) {
		t.Fatalf("golden %s mismatch:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestReadCommandsGolden(t *testing.T) {
	cases := map[string][]string{
		"calendars.txt":         {"calendars"},
		"calendars.json":        {"calendars", "--json"},
		"events.txt":            {"events"},
		"events.json":           {"events", "--json"},
		"events_range.txt":      {"events", "--from", "2026-11-24", "--to", "2027-06-01"},
		"events_sandbox.ndjson": {"events", "--calendar", "Agent Sandbox", "--ndjson", "--fields", "id,title,all_day,recurring"},
		"search.txt":            {"search", "EXAMPLE"},
		"get.txt":               {"get", "E1@" + fmt.Sprint(at(20, 10, 0).UnixMilli())},
		"get_allday.txt":        {"get", "E2"},
		"doctor.txt":            {"doctor"},
	}
	for name, args := range cases {
		code, out, errOut := run(t, newFake(), sandboxRW, "", args...)
		if code != 0 {
			t.Fatalf("%s: exit %d %s", name, code, errOut)
		}
		golden(t, name, out)
	}
}

func TestDefaultRanges(t *testing.T) {
	b := newFake()
	run(t, b, sandboxRW, "", "events")
	if !b.last.From.Equal(at(20, 0, 0)) || !b.last.To.Equal(at(27, 0, 0)) || b.last.Text != "" {
		t.Fatalf("events default: %+v", b.last)
	}
	run(t, b, sandboxRW, "", "events", "--from", "2026-12-01")
	if !b.last.To.Equal(time.Date(2026, 12, 8, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("events --from: %+v", b.last)
	}
	run(t, b, sandboxRW, "", "search", "x")
	if !b.last.From.Equal(at(20, 0, 0).AddDate(0, 0, -30)) || !b.last.To.Equal(at(20, 0, 0).AddDate(0, 0, 365)) || b.last.Text != "x" {
		t.Fatalf("search default: %+v", b.last)
	}
	run(t, b, sandboxRW, "", "search", "x", "--from", "2028-01-01")
	if !b.last.To.Equal(time.Date(2028, 12, 31, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("search late --from: %+v", b.last)
	}
}

func TestReadScopeHidesPrivate(t *testing.T) {
	for _, args := range [][]string{{"calendars"}, {"events"}, {"search", "diary"}} {
		_, out, _ := run(t, newFake(), sandboxRW, "", args...)
		if strings.Contains(out, "Diary") || strings.Contains(out, "Private") {
			t.Fatalf("%v leaked private scope: %s", args, out)
		}
	}
	diary := "E3@" + fmt.Sprint(at(22, 9, 0).UnixMilli())
	if code, _, _ := run(t, newFake(), sandboxRW, "", "get", diary); code != agentcli.ExitPolicy {
		t.Fatalf("get private: %d", code)
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "events", "--calendar", "Private"); code != agentcli.ExitPolicy {
		t.Fatalf("events private: %d", code)
	}
}

func standup(d int) string { return MakeID("S1", at(d, 9, 0).UnixMilli()) }

func TestPolicyDeniedWriteNeverReachesBridge(t *testing.T) {
	b := newFake()
	before := len(b.events)
	for _, args := range [][]string{
		{"add", "--calendar", "Agent Sandbox", "--title", "x", "--start", "today", "--confirm"},
		{"add", "--calendar", "Agent Sandbox", "--title", "x", "--start", "today", "--dry-run"},
		{"update", standup(23), "--title", "y", "--confirm"},
		{"delete", standup(23), "--confirm"},
		{"calendar-create", "Agent New", "--confirm"},
	} {
		code, _, errOut := run(t, b, readOnly, "", args...)
		if code != agentcli.ExitPolicy || !strings.Contains(errOut, "policy_denied") {
			t.Fatalf("%v: exit %d %s", args, code, errOut)
		}
	}
	// Scopes outside the write allowlist are denied even with a write-enabled policy,
	// including the target of a move and the source of a move.
	for _, args := range [][]string{
		{"delete", "E1", "--confirm"},
		{"update", standup(23), "--calendar", "Work", "--confirm"},
		{"update", "E1", "--calendar", "Agent Sandbox", "--confirm"},
		{"add", "--calendar", "Work", "--title", "x", "--start", "today", "--confirm"},
	} {
		if code, _, e := run(t, b, sandboxRW, "", args...); code != agentcli.ExitPolicy {
			t.Fatalf("%v: %d %s", args, code, e)
		}
	}
	if b.writes != 0 || len(b.events) != before {
		t.Fatalf("a denied write reached the bridge (%d writes)", b.writes)
	}
}

func TestWriteFlow(t *testing.T) {
	b := newFake()
	code, out, _ := run(t, b, sandboxRW, "", "add", "--calendar", "Agent Sandbox", "--title", "New", "--start", "2026-12-01T09:00", "--duration", "45m", "--dry-run")
	if code != 0 || !strings.Contains(out, "2026-12-01T09:45:00Z") || b.writes != 0 {
		t.Fatalf("dry-run: %d %s", code, out)
	}
	if code, _, _ := run(t, b, sandboxRW, "", "add", "--calendar", "Agent Sandbox", "--title", "New", "--start", "today"); code != agentcli.ExitConfirm {
		t.Fatalf("missing confirm: %d", code)
	}
	code, out, errOut := run(t, b, sandboxRW, "", "add", "--calendar", "Agent Sandbox", "--title", "New", "--start", "2026-12-24T18:00",
		"--end", "2026-12-24T20:00", "--location", "Home", "--notes", "n", "--url", "https://example.com", "--repeat", "weekly", "--repeat-count", "3", "--confirm", "--json")
	if code != 0 || !strings.Contains(out, `"recurring":true`) || !strings.Contains(out, `"end":"2026-12-24T20:00:00Z"`) {
		t.Fatalf("add: %d %s %s", code, out, errOut)
	}
	in := `{"calendar":"Agent Sandbox","title":"A","start":"2026-12-02","duration":"2d"}` + "\n" +
		`{"calendar":"C2","title":"B","start":"2026-12-03","all_day":true,"end":"2026-12-05T10:00"}`
	code, out, e := run(t, b, sandboxRW, in, "add", "-", "--confirm", "--json")
	if code != 0 || !strings.Contains(out, `"end":"2026-12-03"`) || !strings.Contains(out, `"end":"2026-12-05"`) {
		t.Fatalf("stdin add: %d %s %s", code, out, e)
	}
	id := standup(24)
	if code, _, e := run(t, b, sandboxRW, "", "update", id, "--title", "Moved", "--start", "2026-11-24T11:00", "--location", "L", "--notes", "x", "--url", "", "--span", "this", "--confirm"); code != 0 {
		t.Fatalf("update: %d %s", code, e)
	}
	got, _ := b.Get(context.Background(), id)
	if got.Title != "Moved" || !got.Start.Equal(at(24, 11, 0)) || !got.End.Equal(at(24, 11, 15)) {
		t.Fatalf("update kept duration: %+v", got)
	}
	if code, _, _ := run(t, b, sandboxRW, "", "update", id, "--duration", "1h", "--confirm"); code != 0 {
		t.Fatal("update duration")
	}
	if got, _ := b.Get(context.Background(), id); !got.End.Equal(at(24, 12, 0)) {
		t.Fatalf("duration: %+v", got)
	}
	if code, out, _ := run(t, b, sandboxRW, "", "update", "E2", "--start", "2026-11-28", "--dry-run", "--json"); code != 0 || !strings.Contains(out, `"start":"2026-11-28"`) {
		t.Fatalf("all-day update: %s", out)
	}
	b.cals = append(b.cals, Calendar{ID: "C6", Title: "Agent Other"})
	if code, _, e := run(t, b, sandboxRW, "", "update", id, "--calendar", "Agent Other", "--confirm"); code != 0 {
		t.Fatalf("move: %d %s", code, e)
	}
	if code, _, _ := run(t, b, sandboxRW, "", "update", id, "--calendar", "C6", "--dry-run"); code != agentcli.ExitUsage {
		t.Fatal("moving to the same calendar is no change")
	}
	if code, out, _ := run(t, b, sandboxRW, "", "delete", standup(23), "--span", "future", "--confirm", "--json"); code != 0 || !strings.Contains(out, `"deleted":true`) {
		t.Fatalf("delete: %s", out)
	}
	for _, e := range b.events {
		if e.Title == "Standup" {
			t.Fatalf("future occurrences left: %+v", e)
		}
	}
	if code, _, _ := run(t, b, sandboxRW, "", "calendar-create", "Agent New", "--confirm"); code != 0 {
		t.Fatal("calendar-create")
	}
	if code, _, _ := run(t, b, sandboxRW, "", "calendar-create", "Agent New", "--confirm"); code != agentcli.ExitConflict {
		t.Fatal("duplicate calendar")
	}
}

func TestWriteValidation(t *testing.T) {
	add := func(extra ...string) []string {
		return append([]string{"add", "--calendar", "Agent Sandbox", "--title", "x", "--dry-run"}, extra...)
	}
	cases := map[string][]string{
		"no start":        add(),
		"no title":        {"add", "--calendar", "Agent Sandbox", "--start", "today", "--dry-run"},
		"no end":          add("--start", "2026-12-01T09:00"),
		"end and dur":     add("--start", "2026-12-01T09:00", "--end", "2026-12-01T10:00", "--duration", "1h"),
		"end before":      add("--start", "2026-12-01T09:00", "--end", "2026-12-01T08:00"),
		"allday before":   add("--start", "2026-12-02", "--end", "2026-12-01"),
		"allday hours":    add("--start", "2026-12-02", "--duration", "3h"),
		"bad duration":    add("--start", "2026-12-01T09:00", "--duration", "soon"),
		"zero duration":   add("--start", "2026-12-01T09:00", "--duration", "0m"),
		"huge duration":   add("--start", "2026-12-01T09:00", "--duration", "400d"),
		"bad start":       add("--start", "someday"),
		"bad end":         add("--start", "2026-12-01T09:00", "--end", "x"),
		"bad url":         add("--start", "today", "--url", "javascript:alert(1)"),
		"bad repeat":      add("--start", "today", "--repeat", "hourly"),
		"count no repeat": add("--start", "today", "--repeat-count", "3"),
		"neg count":       add("--start", "today", "--repeat", "daily", "--repeat-count", "-1"),
		"bad count":       add("--start", "today", "--repeat", "daily", "--repeat-count", "x"),
		"bad arg":         {"add", "x", "--dry-run"},
		"nothing":         {"update", "E1", "--dry-run"},
		"empty title":     {"update", "E1", "--title", " ", "--dry-run"},
		"upd bad url":     {"update", "E1", "--url", "ftp://x", "--dry-run"},
		"upd bad start":   {"update", "E1", "--start", "x", "--dry-run"},
		"upd end before":  {"update", "E1", "--end", "2026-11-20T09:00", "--dry-run"},
		"bad span":        {"delete", "E1", "--span", "all", "--dry-run"},
		"empty calendar":  {"calendar-create", " ", "--dry-run"},
		"bad from":        {"events", "--from", "x"},
		"bad to":          {"events", "--to", "x"},
		"to before from":  {"events", "--from", "2026-12-01", "--to", "2026-11-01"},
		"range too long":  {"events", "--from", "2026-01-01", "--to", "2027-01-03"},
		"search too long": {"search", "x", "--from", "2020-01-01", "--to", "2026-01-01"},
		"empty search":    {"search", " "},
	}
	for name, args := range cases {
		if code, _, e := run(t, newFake(), sandboxRW, "", args...); code != agentcli.ExitUsage {
			t.Errorf("%s: exit %d %s", name, code, e)
		}
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "events", "--from", "2026-01-01", "--to", "2027-01-02"); code != 0 {
		t.Error("366 days must be allowed")
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "add", "--calendar", "Dup", "--title", "x", "--start", "today", "--dry-run"); code != agentcli.ExitConflict {
		t.Error("ambiguous calendar name must conflict")
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "add", "--calendar", "Nope", "--title", "x", "--start", "today", "--dry-run"); code != agentcli.ExitNotFound {
		t.Error("unknown calendar")
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "update", "E1", "--calendar", "Nope", "--dry-run"); code != agentcli.ExitNotFound {
		t.Error("unknown move target")
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "delete", standup(23), "nope", "--dry-run"); code != agentcli.ExitNotFound {
		t.Error("unknown id in batch")
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "update", "nope", "--title", "x", "--dry-run"); code != agentcli.ExitNotFound {
		t.Error("unknown id in update")
	}
}

func TestBridgeErrors(t *testing.T) {
	b := newFake()
	b.fail = agentcli.Permissionf("denied")
	for _, args := range [][]string{{"calendars"}, {"events"}, {"search", "x"}, {"events", "--calendar", "Work"}, {"calendar-create", "Agent X", "--dry-run"}} {
		if code, _, _ := run(t, b, sandboxRW, "", args...); code != agentcli.ExitPermission {
			t.Errorf("%v: %d", args, code)
		}
	}
	if code, out, _ := run(t, b, sandboxRW, "", "doctor"); code != agentcli.ExitPermission || !strings.Contains(out, "false") {
		t.Errorf("doctor: %d %s", code, out)
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "get", "nope"); code != agentcli.ExitNotFound {
		t.Error("get missing")
	}
	b2 := newFake()
	b2.fail = agentcli.Unavailablef("x")
	if code, _, e := run(t, b2, sandboxRW, "", "delete", standup(23), standup(24), "--confirm"); code != agentcli.ExitUnavailable || strings.Contains(e, "of 2") {
		t.Errorf("first failure is not partial: %d %s", code, e)
	}
	in := `{"calendar":"Agent Sandbox","title":"A","start":"today"}`
	b3 := newFake()
	b3.fail = agentcli.Unavailablef("x")
	if code, _, _ := run(t, b3, sandboxRW, in, "add", "-", "--confirm"); code != agentcli.ExitPermission && code != agentcli.ExitUnavailable {
		t.Errorf("stdin add failure: %d", code)
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "{", "add", "-", "--confirm"); code != agentcli.ExitUsage {
		t.Error("bad stdin")
	}
}

func TestPartial(t *testing.T) {
	e := agentcli.AsError(partial(agentcli.Unavailablef("boom"), 2, 5))
	if e.Exit != agentcli.ExitUnavailable || !strings.HasPrefix(e.Message, "2 of 5") {
		t.Fatal(e)
	}
}

func TestIDs(t *testing.T) {
	id := MakeID("CAL:uid@example.com", 1795424400000)
	eid, occ := SplitID(id)
	if eid != "CAL:uid@example.com" || occ == nil || *occ != 1795424400000 {
		t.Fatalf("split %q: %q %v", id, eid, occ)
	}
	for _, bare := range []string{"CAL:uid@example.com", "plain", "@123", "x@"} {
		if eid, occ := SplitID(bare); eid != bare || occ != nil {
			t.Errorf("%q: %q %v", bare, eid, occ)
		}
	}
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{"30m": 30 * time.Minute, "1h30m": 90 * time.Minute, "2d": 48 * time.Hour} {
		if d, err := parseDuration(in); err != nil || d != want {
			t.Errorf("%s: %v %v", in, d, err)
		}
	}
	for _, bad := range []string{"", "d", "-1h", "xd", "367d"} {
		if _, err := parseDuration(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSchemaListsWriteOps(t *testing.T) {
	_, out, _ := run(t, newFake(), readOnly, "", "schema")
	var s struct {
		Commands []struct{ Name, Kind, Op string }
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatal(err)
	}
	var writes []string
	for _, c := range s.Commands {
		if c.Kind == "write" {
			writes = append(writes, c.Op)
		}
	}
	if strings.Join(writes, ",") != "add,calendar-create,delete,update" {
		t.Fatalf("write ops: %v", writes)
	}
	if strings.Contains(out, "calendar-delete") {
		t.Fatal("calendar deletion must not be a CLI command")
	}
}

func TestAllDayWithoutEndIsOneDay(t *testing.T) {
	code, out, errOut := run(t, newFake(), sandboxRW, "", "add", "--calendar", "Agent Sandbox", "--title", "Coffee", "--start", "2026-12-01", "--all-day", "--dry-run", "--json")
	if code != 0 || !strings.Contains(out, `"start":"2026-12-01"`) || !strings.Contains(out, `"end":"2026-12-01"`) {
		t.Fatalf("all-day without end: %d %s %s", code, out, errOut)
	}
	if code, _, _ := run(t, newFake(), sandboxRW, "", "add", "--calendar", "Agent Sandbox", "--title", "x", "--start", "2026-12-01T10:00", "--dry-run"); code != agentcli.ExitUsage {
		t.Fatalf("timed event without end must be a usage error: %d", code)
	}
}
