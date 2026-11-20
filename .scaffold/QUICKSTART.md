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
