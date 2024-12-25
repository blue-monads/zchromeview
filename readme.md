# The ChromeView

The ChromeView is a simple wrapper around Chrome in app mode. This allows it to be used as a webview without CGO, eliminating the need to ship the browser with your Go application.

ChromeView reuses the Chrome instance already installed on the system. It uses the Chrome Remote Debugging Protocol to control the Chrome instance.

[Example](./examples/simple/main.go)