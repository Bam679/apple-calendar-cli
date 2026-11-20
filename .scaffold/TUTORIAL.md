## Install the CLI

```sh
brew install Bam679/tap/apple-calendar
apple-calendar version
```

## Give it access to Calendar

```sh
apple-calendar doctor
```

macOS asks whether your terminal may access your calendars. Choose Allow Full Access. The command prints a line with `true` and the number of calendars it can see.

## Read your schedule

```sh
apple-calendar calendars
apple-calendar events
```

The second command lists everything from today until a week from now, one event per line.

## Try to change something

```sh
apple-calendar calendar-create "Agent Tutorial" --dry-run
```

The command exits with code 4: without a policy file the CLI only reads.

## Allow writes in one calendar

```sh
mkdir -p ~/.config/agent-cli
cat > ~/.config/agent-cli/apple-calendar.json <<'JSON'
{
  "version": 1,
  "operations": { "allow": ["read", "write"] },
  "scopes": { "write": { "allow": ["Agent Tutorial"] } }
}
JSON
chmod 600 ~/.config/agent-cli/apple-calendar.json
```

## Add an event

```sh
apple-calendar calendar-create "Agent Tutorial" --confirm
apple-calendar add --calendar "Agent Tutorial" --title "Coffee" --start tomorrow --all-day
```

The `add` command exits with code 5 and shows the event it would create. Run it again with `--confirm`, then look at tomorrow:

```sh
apple-calendar add --calendar "Agent Tutorial" --title "Coffee" --start tomorrow --all-day --confirm
apple-calendar events --calendar "Agent Tutorial" --from tomorrow --to +2d
```

The event is also visible in the Calendar app.

## Clean up

```sh
apple-calendar events --calendar "Agent Tutorial" --from tomorrow --to +2d --fields id
apple-calendar delete <id> --confirm
rm ~/.config/agent-cli/apple-calendar.json
```

The CLI cannot delete calendars, so remove "Agent Tutorial" in the Calendar app (right-click the calendar, then Delete).
