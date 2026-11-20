# ADR-0005: Event ids address one occurrence

- Status: accepted
- Date: 2026-11-20

## Context
EventKit's `eventIdentifier` is shared by every occurrence of a recurring event, so it cannot address "the meeting on Tuesday". `calendarItemIdentifier` has the same problem, and EventKit has no per-occurrence id. When one occurrence is edited (span "this"), EventKit detaches it and gives it the identifier `<series>/RID=<ref>`, and its `hasRecurrenceRules` becomes false. Agents need one id they can copy from `events`/`search` into `get`, `update` and `delete`.

## Decision
- The public id is `<series eventIdentifier>@<occurrenceDate as unix ms>`. The `/RID=…` suffix is stripped, and `occurrenceDate` is the original date, which survives moving one occurrence.
- Go builds and splits ids (`MakeID`/`SplitID`, split at the last `@` because identifiers may contain `@`). The script receives `event_id` and `occurrence_ms` as separate JSON values.
- Resolution: `eventWithIdentifier(event_id)`. A non-recurring event is returned as is and the suffix is ignored, so ids from before a time change still work. For a recurring series the script fetches ±7 days around `occurrence_ms` in that event's calendar and matches the series id plus occurrence date. A bare identifier for a recurring event is a usage error.
- `recurring` is true for series occurrences and detached occurrences.

## Consequences
+ Ids are stable across reads, and across span "this" edits of the same occurrence.
- Changing the start of a non-recurring event changes its id; `update` returns the new one (the old one still resolves).
- A span "future" edit can split the series in EventKit, so later occurrences may get new ids. Agents should re-read after such writes.
- An occurrence moved more than 7 days from its original date is not found by its id.
