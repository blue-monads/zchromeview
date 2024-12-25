package zchromeview

import (
	"os"
	"runtime"
)

type View interface {
	Start() error
	Stop() error
	NavigateURL(url string) error
}

type IPCMessage struct {
	ID        int64  `json:"id"`
	Method    string `json:"method"`
	Params    any    `json:"params"`
	SessionId string `json:"sessionId"`
}

func defaultBinaryPath() string {

	var defaultBinaryPath string

	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/Applications/Google Chrome.app"); err == nil {
			defaultBinaryPath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		}

		if defaultBinaryPath == "" {
			if _, err := os.Stat("/Applications/Brave Browser.app"); err == nil {
				defaultBinaryPath = "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
			}
		}

	} else if runtime.GOOS == "linux" {
		if _, err := os.Stat("/usr/bin/chromium"); err == nil {
			defaultBinaryPath = "/usr/bin/chromium"
		}

		if defaultBinaryPath == "" {
			if _, err := os.Stat("/opt/google/chrome/chrome"); err == nil {
				defaultBinaryPath = "/opt/google/chrome/chrome"
			}
		}

		if defaultBinaryPath == "" {
			if _, err := os.Stat("/usr/bin/brave"); err == nil {
				defaultBinaryPath = "/usr/bin/brave"
			}
		}

	} else if runtime.GOOS == "windows" {
		if _, err := os.Stat("C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe"); err == nil {
			defaultBinaryPath = "C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe"
		}

		if defaultBinaryPath == "" {
			if _, err := os.Stat("C:\\Program Files (x86)\\Chromium\\Application\\chrome.exe"); err == nil {
				defaultBinaryPath = "C:\\Program Files (x86)\\Chromium\\Application\\chrome.exe"
			}
		}

		if defaultBinaryPath == "" {
			if _, err := os.Stat("C:\\Program Files (x86)\\BraveSoftware\\Brave-Browser\\Application\\brave.exe"); err == nil {
				defaultBinaryPath = "C:\\Program Files (x86)\\BraveSoftware\\Brave-Browser\\Application\\brave.exe"
			}
		}

		if defaultBinaryPath == "" {
			if _, err := os.Stat("C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe"); err == nil {
				defaultBinaryPath = "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe"
			}
		}

	}

	return defaultBinaryPath
}

// Chrome Driver types

type TargetInfo struct {
	Attached             bool   `json:"attached"`
	BrowserContextId     string `json:"browserContextId"`
	CanAccessOpener      bool   `json:"canAccessOpener"`
	Title                string `json:"title"`
	TargetId             string `json:"targetId"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
}
