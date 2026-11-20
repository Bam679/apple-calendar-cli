//go:build integration && darwin

package calendar

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Bam679/apple-calendar-cli/internal/agentcli"
)

// The integration test only ever writes to a freshly created calendar named
// agent-cli-test-<random>, enforced by the policy below (read and write scopes
// are that name only) and by DeleteCalendar refusing other titles. Reads always
// pass --calendar so no other calendar's events are fetched. The calendar is
// force-removed in cleanup even if assertions fail. Nothing from the user's
// real calendars is logged.
func TestIntegrationSandboxCalendar(t *testing.T) {
	time.Local = systemLocal
	t.Cleanup(func() { time.Local = time.UTC })
	ctx := context.Background()
	b := NewJXA()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	name := SandboxPrefix + hex.EncodeToString(buf)
	sandbox := agentcli.Effective{Layers: []agentcli.Policy{{Version: 1,
		Operations: agentcli.OpRule{Allow: []string{agentcli.OpRead, agentcli.OpWrite}},
		Scopes: agentcli.ScopeRules{Read: agentcli.Rule{Allow: []string{name}},
			Write: agentcli.Rule{Allow: []string{name}}}}}}
	pol := sandbox
	cli := func(stdin string, args ...string) (int, string) {
		app := NewApp(b, "it")
		app.LoadPolicy = func() (agentcli.Effective, error) { return pol, nil }
		var out, errb bytes.Buffer
		code := app.Main(ctx, args, strings.NewReader(stdin), &out, &errb)
		if code != 0 {
			t.Logf("%s -> %d %s", args[0], code, errb.String())
		}
		return code, out.String()
	}
	decode := func(out string) []map[string]any {
		var v struct{ Items []map[string]any }
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		return v.Items
	}

	if code, _ := cli("", "calendar-create", name, "--confirm"); code != 0 {
		t.Fatal("calendar-create failed")
	}
	t.Cleanup(func() {
		cs, err := b.Calendars(ctx)
		if err != nil {
			t.Errorf("cleanup: %v", err)
		}
		for _, c := range cs {
			if c.Title == name {
				if err := b.DeleteCalendar(ctx, c.ID, true); err != nil {
					t.Errorf("cleanup: %v", err)
				}
			}
		}
	})
	if code, out := cli("", "calendars", "--json"); code != 0 || len(decode(out)) != 1 {
		t.Fatalf("calendars must show only the sandbox: %d", code)
	}

	day := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	hostile := `it-two "quoted" 'x' $(id) ` + "`whoami` \\ </script> ${x} %@ ünï"
	in := `{"calendar":"` + name + `","title":"it-one","start":"` + day + `T09:00","duration":"30m","location":"Room 1"}
{"calendar":"` + name + `","title":` + mustJSON(t, hostile) + `,"start":"` + day + `T11:00","end":"` + day + `T12:15","notes":"line1\nline2","url":"https://example.com/x?a=1&b=2"}
{"calendar":"` + name + `","title":"it-allday","start":"` + day + `","duration":"2d"}`
	code, out := cli(in, "add", "-", "--confirm", "--json")
	if code != 0 {
		t.Fatal("add failed")
	}
	added := decode(out)
	if len(added) != 3 || added[1]["title"] != hostile {
		t.Fatalf("hostile title not preserved: %v", added)
	}
	hid := added[1]["id"].(string)
	if added[2]["all_day"] != true || added[2]["start"] != day || added[2]["end"] == day {
		t.Fatalf("all-day mapping: %v", added[2])
	}

	code, out = cli("", "events", "--calendar", name, "--from", day, "--to", "+5d", "--json")
	if code != 0 || len(decode(out)) != 3 {
		t.Fatalf("events: %s", out)
	}
	if code, out = cli("", "search", "WHOAMI", "--calendar", name); code != 0 || !strings.Contains(out, hid) {
		t.Fatalf("search: %s", out)
	}
	if code, out = cli("", "search", "%@", "--calendar", name, "--json"); code != 0 || len(decode(out)) != 1 {
		t.Fatalf("search must treat format characters as text: %s", out)
	}
	if code, out = cli("", "get", hid, "--json"); code != 0 || !strings.Contains(out, `line1\nline2`) || !strings.Contains(out, "a=1&b=2") {
		t.Fatalf("get: %s", out)
	}
	newStart := day + "T14:00"
	code, out = cli("", "update", hid, "--title", "it-renamed", "--start", newStart, "--location", "L2", "--confirm", "--json")
	if code != 0 {
		t.Fatal("update")
	}
	u := decode(out)[0]
	st, _ := time.Parse(time.RFC3339, u["start"].(string))
	en, _ := time.Parse(time.RFC3339, u["end"].(string))
	if u["title"] != "it-renamed" || st.Hour() != 14 || en.Sub(st) != 75*time.Minute || u["location"] != "L2" {
		t.Fatalf("after update: %v", u)
	}
	hid = u["id"].(string)
	if _, err := b.Get(ctx, hid); err != nil {
		t.Fatalf("updated id does not resolve: %v", err)
	}

	// Recurring: 4 daily occurrences, then span=this and span=future writes.
	rec := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	if code, _ = cli("", "add", "--calendar", name, "--title", "it-daily", "--start", rec+"T08:00", "--duration", "15m",
		"--repeat", "daily", "--repeat-count", "4", "--confirm"); code != 0 {
		t.Fatal("recurring add")
	}
	occs := func() []map[string]any {
		_, out := cli("", "search", "it-daily", "--calendar", name, "--from", rec, "--to", "+20d", "--json")
		return decode(out)
	}
	all := occs()
	if len(all) != 4 || all[0]["recurring"] != true || all[0]["id"] == all[1]["id"] {
		t.Fatalf("occurrences: %d", len(all))
	}
	if eidA, _ := SplitID(all[0]["id"].(string)); !strings.HasPrefix(all[1]["id"].(string), eidA+"@") {
		t.Fatal("occurrences must share the event identifier")
	}
	if code, _ = cli("", "update", all[1]["id"].(string), "--title", "it-daily-moved", "--span", "this", "--confirm"); code != 0 {
		t.Fatal("update span this")
	}
	if code, _ = cli("", "get", all[1]["id"].(string)); code != 0 {
		t.Fatal("occurrence id must stay stable after a span=this update")
	}
	if code, out = cli("", "events", "--calendar", name, "--from", rec, "--to", "+20d", "--json"); code != 0 {
		t.Fatal("events after span update")
	}
	moved := 0
	for _, it := range decode(out) {
		if it["title"] == "it-daily-moved" {
			moved++
		}
	}
	if moved != 1 {
		t.Fatalf("span=this changed %d occurrences", moved)
	}
	if code, _ = cli("", "delete", all[2]["id"].(string), "--span", "future", "--confirm"); code != 0 {
		t.Fatal("delete span future")
	}
	// Occurrences 0 and 1 (the detached "it-daily-moved") remain.
	if left := occs(); len(left) != 2 {
		t.Fatalf("after delete future: %d it-daily occurrences", len(left))
	}

	// A policy-denied write against the real bridge must not reach EventKit.
	pol = agentcli.Effective{Layers: []agentcli.Policy{agentcli.DefaultPolicy()}}
	if code, _ = cli("", "delete", hid, "--confirm"); code != agentcli.ExitPolicy {
		t.Fatalf("denied delete: %d", code)
	}
	if _, err := b.Get(ctx, hid); err != nil {
		t.Fatal("denied delete removed the event")
	}
	pol = sandbox

	if code, _ = cli("", "delete", hid, "--confirm"); code != 0 {
		t.Fatal("delete")
	}
	if _, err := b.Get(ctx, hid); agentcli.AsError(err).Exit != agentcli.ExitNotFound {
		t.Fatalf("deleted event still resolves: %v", err)
	}
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
