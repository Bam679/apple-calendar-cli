# apple-calendar CLI reference

`apple-calendar` version 0.x. `apple-calendar schema` prints the same information as JSON and is always current for the installed version.

## Synopsis

```text
apple-calendar <command> [arguments] [flags]
```

Flags can appear before or after arguments. Every flag has two dashes. `--` ends flag parsing.

## Commands

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

## Global flags

| Flag | Type | Description |
|---|---|---|
| `--json` | bool | Print one JSON document. |
| `--ndjson` | bool | Print one JSON object per line. |
| `--fields a,b` | string | Select and order fields. Unknown names are a usage error. |
| `--limit N` | int | Maximum number of items. Default 50, at most 1000. |
| `--cursor C` | string | Continue a list from a `next_cursor`. A cursor only works with the same command and flags. |
| `--max-chars N` | int | Cap the size of stdout. Lists drop whole items, single items shorten their longest text field. |
| `--dry-run` | bool | Writes only. Print the plan, change nothing. |
| `--confirm` | bool | Writes only. Apply the change. |
| `--timeout D` | duration | Abort after this time (default `60s`). |
| `--help` | bool | Show help. |

## Output formats

| Format | Lists | Single items |
|---|---|---|
| default | One line per item, tab-separated default fields, `-` for empty values. Trailer lines start with `#`: `# next_cursor=…`, `# truncated: …` | `field: value` lines |
| `--json` | `{"items":[…],"count":N,"next_cursor":"…","truncated":true}` | One object; `"truncated":true` when shortened |
| `--ndjson` | One object per line, then `{"_meta":{"next_cursor":…,"truncated":…}}` when relevant | One object |

Write commands print the changed items with an `op` field. With `--dry-run`, or without `--confirm`, they print the planned items with `"dry_run":true`.

Text output never contains control characters, tabs or newlines inside a value, or bidirectional override characters.

Errors are one JSON line on stderr:

```json
{"error":{"code":"policy_denied","message":"writing to \"Private\" is not allowed by policy","hint":"…","exit":4}}
```

## Exit codes

| Code | Name | Meaning |
|---|---|---|
| 0 | ok | Success. |
| 1 | internal | Unexpected error. |
| 2 | usage | Invalid arguments or flags. |
| 3 | not_found | The item or container does not exist. |
| 4 | policy_denied | The policy does not allow the operation or scope, or the policy file is invalid. No flag overrides this. |
| 5 | confirm_required | A write without `--confirm`. The plan is printed on stdout. |
| 6 | permission_denied | Missing macOS privacy permission, or invalid credentials. |
| 7 | unavailable | The backend did not answer, or the timeout expired. |
| 8 | conflict | An ambiguous name, or the item already exists. |

## Stdin

Write commands that take `-` as an argument read JSON objects from stdin: one object, an array of objects, or one object per line. Input is limited to 4 MiB, and unknown fields are rejected.

## Dates

Date flags accept `YYYY-MM-DD` (a whole day), `YYYY-MM-DDTHH:MM` and `YYYY-MM-DD HH:MM` in local time, RFC 3339, `today`, `tomorrow`, `yesterday`, `+Nd` and `-Nd`.

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

## Policy file

Path: `~/.config/agent-cli/apple-calendar.json`. The home directory comes from the user database, not from `$HOME`. No flag or environment variable selects a different file.

| Key | Type | Meaning |
|---|---|---|
| `version` | int | Must be `1`. |
| `operations.allow` | string list | `read`, `write` (every write operation), or single operations: `add`, `update`, `delete`, `calendar-create`. |
| `operations.deny` | string list | Denied operations. Deny wins over allow. |
| `scopes.read.allow` | glob list | Containers that may be read. Empty means all. |
| `scopes.read.deny` | glob list | Containers hidden from every read. |
| `scopes.write.allow` | glob list | Containers that may be changed. Empty means none. |
| `scopes.write.deny` | glob list | Containers that may never be changed. |
| `limits.max_write_batch` | int | Maximum number of items one write command may change (default 100). |

Scope strings are calendar titles (e.g. `Work`, `Agent*`). In patterns `*` matches any text, including `/`; matching is case-sensitive.

Without a file, the default policy allows `read` on everything and no writes. The CLI rejects files that are not regular files, that are writable by group or others, that another user owns, or that contain unknown keys or invalid JSON. A rejected file denies every command except `schema`, `policy`, `version`, `help` and `guard`.

## Built-in commands

| Command | Description |
|---|---|
| `schema` | The machine-readable contract as JSON. |
| `policy` | The effective policy and its path. `--example` prints a template, and `--claude-settings` prints recommended Claude Code settings. |
| `guard` | Claude Code `PreToolUse` hook. It reads the hook JSON on stdin and denies tool calls that would modify agent-cli policy files. |
| `version` | Name and version. |
| `help` | Usage. |
