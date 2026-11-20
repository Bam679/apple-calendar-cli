# apple-calendar

Read and manage Apple Calendar events from the command line, built for AI coding agents.

[![ci](https://github.com/Bam679/apple-calendar-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/Bam679/apple-calendar-cli/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/Bam679/apple-calendar-cli)](https://github.com/Bam679/apple-calendar-cli/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Bam679/apple-calendar-cli/badge)](https://scorecard.dev/viewer/?uri=github.com/Bam679/apple-calendar-cli)
[![licence](https://img.shields.io/badge/licence-MIT-blue.svg)](LICENSE)

`apple-calendar` reads and writes events through EventKit, the macOS framework behind the Calendar app, and prints one line per event or JSON. Everything runs on your Mac, and the CLI itself makes no network connections. Writes stay off until a policy file allows them, and attendee addresses are never printed.



## Why a CLI instead of an MCP server

An MCP server loads its tool definitions into the agent's context in every session, and each result travels back through the model before the agent can act on it. A CLI costs nothing until it is called. The agent learns it from a short skill, filters the output with `jq` or `grep`, and chains several steps in one shell script, for example turning this week's events into reminders in a single run.

`apple-calendar` is one of five CLIs with the same output format, exit codes and permission model: [apple-mail](https://github.com/Bam679/apple-mail-cli), [apple-notes](https://github.com/Bam679/apple-notes-cli), [apple-calendar](https://github.com/Bam679/apple-calendar-cli), [apple-reminders](https://github.com/Bam679/apple-reminders-cli) and [paperless](https://github.com/Bam679/paperless-cli). Together they let an agent manage everyday things on a Mac with one set of rules. If your agent cannot run shell commands, an MCP server is the better fit.

## Install

1. Install the CLI:

   ```sh
   brew install Bam679/tap/apple-calendar
   ```

   Signed release archives and `go install` are described in [How to install and verify a release](docs/how-to/install-and-verify.md).

2. Check that it can reach Calendar:

   ```sh
   apple-calendar doctor
   ```

   On the first run macOS asks whether your terminal app may access your calendars; choose full access. See [Security and permissions](#security-and-permissions).

3. Connect Claude Code. The plugin adds a skill that teaches the agent the CLI, and a hook that stops the agent from editing the policy file:

   ```sh
   claude plugin marketplace add Bam679/apple-calendar-cli
   claude plugin install apple-calendar@apple-calendar-cli
   ```

   To get all five tools and the shared `agent-cli` skill from one place, use the [agent-cli-plugins](https://github.com/Bam679/agent-cli-plugins) marketplace instead. Install each plugin from one marketplace only.

To remove everything: `claude plugin uninstall apple-calendar`, `brew uninstall apple-calendar`, and delete `~/.config/agent-cli/apple-calendar.json` if you created one.

## Quick start

```sh
apple-calendar doctor
```

```text
calendar_access	true	12 calendars visible
```

```sh
apple-calendar calendars                              # one line per calendar
apple-calendar events                                 # today until a week from now
apple-calendar search dentist --json --fields id,start,title
```

Without a policy file every write is refused, a dry run included:

```sh
apple-calendar add --calendar Agent --title Test --start tomorrow --all-day --dry-run
```

```text
{"error":{"code":"policy_denied","message":"operation \"add\" is not allowed by policy","hint":"the policy file must allow this; agents must not edit it","exit":4}}
```

The [tutorial](docs/tutorial/getting-started.md) continues from here to a first write in a container made for the exercise.

## Use it with your agent

With the plugin installed, Claude Code picks up the skill on its own. For other agents (Codex, Cursor, Gemini CLI), add this to `AGENTS.md`:

```markdown
Use `apple-calendar` for the user's calendars and events. Run `apple-calendar schema` once to learn the commands.
Writes need the user's policy file: preview with `--dry-run`, then run with `--confirm`
only after the user agreed. On exit code 4, stop and tell the user.
```

What an agent can rely on: the default output has one item per line with tab-separated fields; `--json` prints `{"items":[…],"count":N,"next_cursor":"…"}` and `--ndjson` one object per line. `--fields`, `--limit` (default 50, at most 1000) and `--max-chars` keep output small, and `--cursor` continues a list. Errors are one JSON line on stderr, and exit codes are fixed:

| Exit | Meaning |
|---|---|
| 0 | success |
| 1 | unexpected error |
| 2 | invalid arguments or flags |
| 3 | item or container not found |
| 4 | denied by the policy; no flag overrides this |
| 5 | write without `--confirm`; the plan is printed, nothing changed |
| 6 | missing macOS permission or credentials |
| 7 | backend did not answer in time |
| 8 | ambiguous name, or the item already exists |

The [CLI reference](docs/reference/cli.md) has every command, flag, field and the JSON shapes.

## How it works

`apple-calendar` runs one fixed JXA script through `/usr/bin/osascript`. The script talks to EventKit and receives your input as a single JSON argument, so text from you or the agent never becomes code. Event identifiers combine the series identifier with the occurrence date, so an agent can address one occurrence of a recurring meeting. Every write command first builds a plan. The binary checks the plan against the policy file, then prints it (`--dry-run`, or no `--confirm`) or applies it (`--confirm`). [Security model](docs/explanation/security-model.md) explains why the checks live in the binary.

## When not to use it

- You need to send invitations or answer them. EventKit can't, and the CLI doesn't try.
- You need to delete calendars. The CLI only creates them; delete them in the Calendar app.
- Your agent cannot run shell commands. An MCP server fits better there.

## Security and permissions

`apple-calendar` only reads until you allow writes in `~/.config/agent-cli/apple-calendar.json`:

```json
   {
     "version": 1,
     "operations": {
       "allow": [
         "read",
         "add",
         "update"
       ],
       "deny": [
         "delete",
         "calendar-create"
       ]
     },
     "scopes": {
       "read": {
         "allow": [
           "*"
         ],
         "deny": [
           "Private*"
         ]
       },
       "write": {
         "allow": [
           "Agent*"
         ],
         "deny": []
       }
     },
     "limits": {
       "max_write_batch": 10
     }
   }
```

Operations are `read`, `write` (all writes) or single operations: `add`, `update`, `delete`, `calendar-create`. Scopes are glob patterns over calendar titles (e.g. `Work`, `Agent*`). Deny wins over allow, and an empty `scopes.read.allow` means everything may be read. `apple-calendar policy` shows what is allowed now.

The binary reads the policy from that fixed path only. It takes the home directory from the user database, not from `$HOME`, and no flag or environment variable points it elsewhere. A file that is invalid, writable by other users or owned by someone else makes every command fail with exit code 4.

`apple-calendar policy --claude-settings` prints Claude Code settings that allow read commands, ask before every write, deny access to the policy directory, and turn on the Bash sandbox with only `apple-calendar` excluded. In the sandbox the agent cannot reach Calendar through `osascript`, the network or the policy file directly, so `apple-calendar` and its policy are the only way to the data. [How to configure permissions](docs/how-to/configure-permissions.md) walks through it.

### macOS privacy permission

`apple-calendar` reaches Calendar through EventKit, which it runs via `/usr/bin/osascript`. macOS gives the permission to the app that runs the CLI: Terminal, iTerm, VS Code or Claude. On first use macOS asks for Calendars access. If you declined, open System Settings > Privacy & Security > Calendars and choose Full Access for your terminal app; write-only access is not enough for reading. `apple-calendar doctor` shows whether access works; without it, commands exit with code 6.

### What leaves your machine

`apple-calendar` makes no network connections. Events stay in EventKit on your Mac, and your calendar accounts sync them as usual. There is no telemetry and no update check. Whatever an agent reads through `apple-calendar` is sent to the model provider by the agent, like any other tool output.

### Threat model

| Threat | Mitigation |
|---|---|
| A prompt-injected agent tries destructive writes | Writes are off by default. The binary checks an operation and scope allowlist, needs `--confirm` and limits batch sizes. |
| The agent edits its own permissions | The policy path is fixed. The plugin hook and the recommended deny rules block edits, and the sandbox blocks writes outside the project. |
| Input turns into code | Static JXA script (`go:embed`). Input travels only as one JSON argv element to `/usr/bin/osascript` (absolute path), and fuzz tests check the script is never altered by input. Search text reaches EventKit only as an `NSPredicate` argument, never as format text. An integration test round-trips hostile strings through real osascript. |
| Text in the data steers the agent | Output is data only and control characters are stripped. The skill tells agents to treat returned content as untrusted, and read scopes can hide containers. |
| Secrets or personal data leak | No credentials involved; macOS TCC controls access. Errors never contain credentials. |
| Supply chain | No third-party Go dependencies. Actions are pinned to commit SHAs. CI runs govulncheck, gosec, CodeQL, gitleaks, dependency review and OpenSSF Scorecard. Releases ship signed checksums, SBOMs and build provenance. |

An agent with unrestricted shell access can go around any CLI, for example by calling `osascript` or `curl` directly. Only the sandbox closes that gap.

### Protecting PII with AI agents

- Hide sensitive containers with `scopes.read.deny`. Hidden containers never appear in the output.
- Ask for less. `--fields`, `--limit` and `--max-chars` keep personal data out of the context, and the skill tells agents to use them.
- Mask personal data before it reaches the model. [PasqualePillitteri/claude-privacy-tool](https://github.com/PasqualePillitteri/claude-privacy-tool) adds an offline masking hook to Claude Code, and [gregmos/PII-Shield](https://github.com/gregmos/PII-Shield) anonymises PII through an MCP server, a skill or a CLI.
- Scan tool output with a `PostToolUse` hook: [gitleaks/gitleaks](https://github.com/gitleaks/gitleaks) and [trufflesecurity/trufflehog](https://github.com/trufflesecurity/trufflehog) find secrets, and [data-privacy-stack/presidio](https://github.com/data-privacy-stack/presidio) (formerly microsoft/presidio) finds names, addresses, IBANs and phone numbers.
- Keep real data out of bug reports. Redact output or use synthetic examples.

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

## Documentation

| Page | Use it when |
|---|---|
| [Getting started](docs/tutorial/getting-started.md) | you are new and want a first read and a first write |
| [How to configure permissions](docs/how-to/configure-permissions.md) | you want an agent to write, or to hide private data |
| [How to install and verify a release](docs/how-to/install-and-verify.md) | you install without Homebrew or want to check signatures |
| [CLI reference](docs/reference/cli.md) | you need a flag, a field, an exit code or the policy format |
| [Security model](docs/explanation/security-model.md) | you want to know why the checks work this way and where they end |
| [Architecture decisions](docs/adr) | you want the reasoning behind the design |

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) has the rules for changes, and [SECURITY.md](SECURITY.md) explains how to report a vulnerability privately.

```sh
git clone https://github.com/Bam679/apple-calendar-cli && cd apple-calendar-cli
lefthook install   # gofmt, gitleaks and commit-message checks
make check         # everything CI runs
make integration   # tests against the real backend on this machine
```

## Licence

MIT. See [LICENSE](LICENSE).
