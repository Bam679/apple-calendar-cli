## macOS privacy permission

`apple-calendar` reaches Calendar through EventKit, which it runs via `/usr/bin/osascript`. macOS gives the permission to the app that runs the CLI: Terminal, iTerm, VS Code or Claude. On first use macOS asks for Calendars access. If you declined, open System Settings > Privacy & Security > Calendars and choose Full Access for your terminal app; write-only access is not enough for reading. `apple-calendar doctor` shows whether access works; without it, commands exit with code 6.
