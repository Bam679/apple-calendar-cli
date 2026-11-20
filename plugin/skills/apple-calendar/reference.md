# apple-calendar reference

## Fields
`id, title, calendar, calendar_id, start, end, all_day, location, notes, url, recurring, status, organizer, attendees`

- `start`/`end` are `YYYY-MM-DD` for all-day events (the `end` is the last day, inclusive) and RFC 3339 (local offset) otherwise.
- `id` is `<eventIdentifier>@<occurrence ms>` and addresses one occurrence (see docs/adr/0005-event-ids.md).
- `recurring` is true for occurrences of a series, including ones edited on their own.
- `status` is `none|confirmed|tentative|canceled`. `organizer` is a display name.
- `attendees` is a count only. Attendee emails are never output.
- Calendars: `id, title, account, writable, color` (`color` is `#rrggbb`).

## Commands
| command | policy op | notes |
|---|---|---|
| `calendars` | read | calendars hidden by `scopes.read.deny` are omitted |
| `events` | read | `--calendar` (repeatable, name or id), `--from` (default today 00:00), `--to` (exclusive, default `--from` +7d). Max 366 days. Events overlapping the range, sorted by start |
| `search <text>` | read | case-insensitive match on title, location or notes; default range -30d to +365d (max 4 years) |
| `get <id>` | read | exit 4 if the event's calendar is read-denied |
| `add` | add | `--calendar --title --start` required; `--end` or `--duration` for timed events; `--all-day`, `--location`, `--notes`, `--url`, `--repeat daily\|weekly\|monthly\|yearly`, `--repeat-count N`. `-` reads JSON objects `{calendar,title,start,end,duration,all_day,location,notes,url,repeat,repeat_count}` |
| `update <id>` | update | `--title --start --end --duration --location --notes --url --calendar --span this\|future`. A new `--start` keeps the duration. `--calendar` moves the event (both calendars must be write-allowed) |
| `delete <id>…` | delete | `--span this\|future` |
| `calendar-create <title>` | calendar-create | conflict if a calendar with that title exists |

## Policy (written by the user, read-only for agents)
`~/.config/agent-cli/apple-calendar.json`. Scopes match **calendar titles**. Example: allow adding and updating events in `Agent*` calendars only:
```json
{"version":1,"operations":{"allow":["read","add","update"]},"scopes":{"read":{"allow":["*"]},"write":{"allow":["Agent*"]}}}
```
`apple-calendar policy` shows the effective policy and `apple-calendar policy --example` prints a template. Agents must not create or edit this file; ask the user instead.

## Batches
`delete` and `add -` act on several items. `limits.max_write_batch` caps the batch size. If a batch fails part-way, the error message starts with `N of M writes succeeded`.
