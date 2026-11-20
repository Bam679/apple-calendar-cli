```sh
apple-calendar events                                     # this week (one line each)
apple-calendar events --calendar Work --from tomorrow --to +2d --json --fields id,start,title
apple-calendar search dentist --json --fields id,start,title
apple-calendar add --calendar "Agent" --title "Call Bob" --start 2026-12-01T09:00 --duration 30m --dry-run
apple-calendar add --calendar "Agent" --title "Call Bob" --start 2026-12-01T09:00 --duration 30m --confirm
printf '%s\n' '{"calendar":"Agent","title":"Focus","start":"2026-12-02T08:00","duration":"2h"}' \
  | apple-calendar add - --confirm --ndjson                 # batch via stdin
apple-calendar update 'AB12…:CD34@1795424400000' --start 2026-12-01T10:00 --span this --confirm
apple-calendar events --limit 20 --cursor "$(apple-calendar events --limit 20 --json | jq -r .next_cursor)"
```
