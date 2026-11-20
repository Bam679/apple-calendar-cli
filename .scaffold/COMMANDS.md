| Command | Kind | Description |
|---|---|---|
| `calendars` | read | calendars (id, title, account, writable, color) |
| `events [--calendar C]… [--from D] [--to D]` | read | events overlapping the range (default: today 00:00 to +7d, max 366 days), sorted by start |
| `search <text> [--calendar C]… [--from D] [--to D]` | read | title/location/notes contain text (case-insensitive; default range -30d to +365d) |
| `get <id>` | read | one event occurrence with all fields |
| `doctor` | read | permission check |
| `add --calendar C --title T --start D (--end D\|--duration 1h) [--all-day] [--location] [--notes] [--url] [--repeat F --repeat-count N]` / `add -` | write `add` | create events (stdin: JSON/NDJSON) |
| `update <id> [--title] [--start] [--end\|--duration] [--location] [--notes] [--url] [--calendar] [--span this\|future]` | write `update` | change an event or a recurring series from this occurrence on |
| `delete <id>… [--span this\|future]` | write `delete` | delete event occurrences (or this and all future ones) |
| `calendar-create <title>` | write `calendar-create` | create a calendar in the default account |
| `schema`, `policy`, `guard`, `version` | builtin | contract, effective policy, hook guard, version |

Dates accept `YYYY-MM-DD` (all-day), `YYYY-MM-DDTHH:MM`, RFC 3339, `today`, `tomorrow` and `+Nd`. Durations accept `30m`, `1h30m` or `2d`. Event ids look like `<eventIdentifier>@<occurrence ms>` and address one occurrence (see [ADR-0005](docs/adr/0005-event-ids.md)). Calendars cannot be deleted through the CLI.
