## Fields

| Field | Description |
|---|---|
| `id` | `<eventIdentifier>@<occurrence start in Unix ms>`. It addresses one occurrence of a recurring event and stays stable when that occurrence is edited ([ADR-0005](../adr/0005-event-ids.md)). |
| `title`, `location`, `notes`, `url` | Text fields. |
| `calendar`, `calendar_id` | Title and identifier of the calendar. |
| `start`, `end`, `all_day` | RFC 3339 in local time; all-day events use `YYYY-MM-DD`. |
| `recurring` | `true` for occurrences of a series. |
| `status`, `organizer` | Event status and organiser name. |
| `attendees` | Number of attendees. Addresses are not printed. |

The default text columns are `id start end title calendar` for events and `id title account` for calendars. Events are sorted by start time.

## Scopes

Policy scopes match calendar titles. Moving an event with `update --calendar` needs write scope on both calendars.

## Behaviour notes

- `events` covers today 00:00 to seven days ahead by default; a range longer than 366 days is a usage error. `search` covers 30 days back to a year ahead.
- Durations accept `30m`, `1h30m` or `2d`. `add --repeat daily|weekly|monthly|yearly --repeat-count N` creates a series.
- `--span future` changes or deletes this occurrence and all later ones. After such a write, and after changing the start of an event, the output carries the new identifier.
- Calendars cannot be deleted through the CLI.
