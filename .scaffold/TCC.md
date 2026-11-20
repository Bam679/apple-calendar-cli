## macOS privacy (TCC)

`apple-calendar` uses EventKit through `/usr/bin/osascript`. macOS attributes the access to the **app that runs the CLI** (Terminal, iTerm, VS Code, Claude desktop …). On first use, macOS asks for **Calendars** access. If you denied it, enable your terminal app under **System Settings > Privacy & Security > Calendars** and choose **Full Access** (write-only access is not enough for reads). `apple-calendar doctor` reports the status, and errors use exit code 6 with a hint.
