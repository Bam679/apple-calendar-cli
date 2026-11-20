package calendar

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Bam679/apple-calendar-cli/internal/agentcli"
	"github.com/Bam679/apple-calendar-cli/internal/osa"
)

func TestScriptIsStatic(t *testing.T) {
	if !strings.Contains(script, "const OPS") || strings.Contains(script, "%s") || strings.Contains(script, "%v") {
		t.Fatal("script must be a static OPS table")
	}
	for _, op := range []string{"calendars", "events", "get", "add", "update", "delete", "calendar_create", "calendar_delete"} {
		if !strings.Contains(script, "  "+op+": function") {
			t.Errorf("script lacks op %s", op)
		}
	}
	if !strings.Contains(script, "predicateWithFormatArgumentArray(SEARCH_FORMAT,") {
		t.Error("search text must only be a predicate argument")
	}
}

func TestJXAWireMapping(t *testing.T) {
	start := int64(1795424400000) // 2026-11-23T09:00:00Z
	occ := start - 3600000
	wire := map[string]any{"event_id": "CAL:uid@example.com", "occurrence_ms": occ, "title": "T", "calendar": "C",
		"calendar_id": "CID", "start_ms": start, "end_ms": start + 900000, "all_day": false, "location": "L",
		"notes": "n", "url": "https://e.x", "recurring": true, "status": "tentative", "organizer": "O", "attendees": 2}
	noOcc := map[string]any{"event_id": "E", "occurrence_ms": nil, "start_ms": start, "end_ms": nil}
	seen := map[string]json.RawMessage{}
	f := &osa.Fake{Handler: func(op string, args json.RawMessage) (any, error) {
		seen[op] = args
		switch op {
		case "calendars":
			return []Calendar{{ID: "1", Title: "A"}, {ID: "S", Title: SandboxPrefix + "x"}}, nil
		case "calendar_create":
			return Calendar{ID: "2", Title: "B", Color: "#ffffff"}, nil
		case "events":
			return []any{wire, noOcc}, nil
		case "delete", "calendar_delete":
			return map[string]string{"id": "X"}, nil
		}
		return wire, nil
	}}
	j := &JXA{Runner: f}
	ctx := context.Background()
	cs, err := j.Calendars(ctx)
	if err != nil || len(cs) != 2 {
		t.Fatal(err)
	}
	from := time.UnixMilli(start)
	es, err := j.Events(ctx, Query{CalendarIDs: []string{"a"}, From: from, To: from.Add(time.Hour), Text: "q"})
	if err != nil || len(es) != 2 {
		t.Fatalf("events: %v", err)
	}
	e := es[0]
	if e.ID != "CAL:uid@example.com@1795420800000" || !e.Start.Equal(from) || e.End.Sub(e.Start) != 15*time.Minute ||
		!e.Recurring || e.Status != "tentative" || e.Attendees != 2 || e.Organizer != "O" {
		t.Fatalf("mapping: %+v", e)
	}
	if es[1].ID != MakeID("E", start) || !es[1].End.IsZero() {
		t.Fatalf("fallback id: %+v", es[1])
	}
	if s := string(seen["events"]); !strings.Contains(s, `"from_ms":1795424400000`) || !strings.Contains(s, `"query":"q"`) {
		t.Fatalf("events args: %s", s)
	}
	if _, err := j.Get(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if s := string(seen["get"]); s != `{"event_id":"CAL:uid@example.com","occurrence_ms":1795420800000}` {
		t.Fatalf("get args: %s", s)
	}
	if _, err := j.Add(ctx, NewEvent{CalendarID: "C", Title: "T", Start: from, End: from.Add(time.Hour), Repeat: "daily", RepeatCount: 3}); err != nil {
		t.Fatal(err)
	}
	if s := string(seen["add"]); !strings.Contains(s, `"start_ms":1795424400000`) || !strings.Contains(s, `"repeat":"daily"`) || !strings.Contains(s, `"repeat_count":3`) {
		t.Fatalf("add args: %s", s)
	}
	title := "new"
	if _, err := j.Update(ctx, "E", Patch{Title: &title}, SpanFuture); err != nil {
		t.Fatal(err)
	}
	if s := string(seen["update"]); !strings.Contains(s, `"start_ms":null`) || !strings.Contains(s, `"span":"future"`) ||
		!strings.Contains(s, `"occurrence_ms":null`) || !strings.Contains(s, `"notes":null`) || !strings.Contains(s, `"title":"new"`) {
		t.Fatalf("update args: %s", s)
	}
	if err := j.Delete(ctx, e.ID, SpanThis); err != nil || !strings.Contains(string(seen["delete"]), `"span":"this"`) {
		t.Fatal(err, string(seen["delete"]))
	}
	if _, err := j.CreateCalendar(ctx, "B"); err != nil {
		t.Fatal(err)
	}
	if err := j.DeleteCalendar(ctx, "S", true); err != nil || !strings.Contains(string(seen["calendar_delete"]), `"force":true`) {
		t.Fatal(err)
	}
	if NewJXA().Runner == nil {
		t.Fatal("production runner")
	}
}

func TestDeleteCalendarOnlySandbox(t *testing.T) {
	f := &osa.Fake{Handler: func(op string, _ json.RawMessage) (any, error) {
		if op == "calendars" {
			return []Calendar{{ID: "1", Title: "Work"}}, nil
		}
		t.Fatalf("unexpected op %s", op)
		return nil, nil
	}}
	j := &JXA{Runner: f}
	if err := j.DeleteCalendar(context.Background(), "1", true); agentcli.AsError(err).Exit != agentcli.ExitPolicy {
		t.Fatalf("non-sandbox delete: %v", err)
	}
	if err := j.DeleteCalendar(context.Background(), "nope", true); agentcli.AsError(err).Exit != agentcli.ExitNotFound {
		t.Fatalf("missing: %v", err)
	}
}

func TestJXAErrorPropagates(t *testing.T) {
	f := &osa.Fake{Handler: func(string, json.RawMessage) (any, error) { return nil, context.DeadlineExceeded }}
	j := &JXA{Runner: f}
	if _, err := j.Events(context.Background(), Query{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := j.Get(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
	if err := j.DeleteCalendar(context.Background(), "x", false); err == nil {
		t.Fatal("expected error")
	}
}
