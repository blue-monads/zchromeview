package zchromeview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/gorilla/websocket"
)

const (
	StateModeNormal       = "shared"
	StateModeLocalFolder  = "local-folder"
	StateModeTempFolder   = "temp-folder"
	StateModeCustomFolder = "custom-folder"
	StateModeIsolated     = "isolated"
)

type Options struct {
	Name         string
	PathToBinary string
	StateMode    string
	StatePath    string // only used if StateMode is StateModeCustomFolder
	StartUpURL   string
	ExtraArgs    []string
}

type ZChromeView struct {
	opts    Options
	profile string

	debuggerURL string
	wsConn      *websocket.Conn
	msgChan     chan *IPCMessage
	cmd         *exec.Cmd

	msgIdCounter int64
}

func New(opts Options) *ZChromeView {

	if opts.PathToBinary == "" {
		opts.PathToBinary = defaultBinaryPath()
	}

	profile := ""

	if opts.StateMode == "" {
		opts.StateMode = StateModeNormal
	}

	if opts.StateMode == StateModeTempFolder {
		profile = os.TempDir()
	} else if opts.StateMode == StateModeLocalFolder {
		// working directory
		dir, _ := os.Getwd()
		profile = fmt.Sprintf("%s/.zchromeview", dir)

	} else if opts.StateMode == StateModeCustomFolder {
		profile = opts.StatePath
	} else if opts.StateMode == StateModeIsolated {
		profile = path.Join(xdg.DataHome, "zchromeview", opts.Name)
	}

	return &ZChromeView{
		opts:         opts,
		profile:      profile,
		debuggerURL:  "",
		wsConn:       nil,
		msgChan:      make(chan *IPCMessage, 2),
		msgIdCounter: 0,
	}
}

func (z *ZChromeView) Start() error {

	args := z.generateArgs()

	if z.profile != "" {
		if err := os.MkdirAll(z.profile, 0755); err != nil {
			return fmt.Errorf("failed to create profile directory: %v", err)
		}
	}

	slog.Info("starting chrome", "args", slog.String("binary", z.opts.PathToBinary), slog.String("args", strings.Join(args, " ")))

	cmd := exec.Command(z.opts.PathToBinary, args...)
	z.cmd = cmd

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	slog.Info("before start")

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start chrome: %v", err)
	}

	wsURLChan := make(chan string, 1)
	errChan := make(chan error, 1)
	timer := time.NewTimer(10 * time.Second)

	go func() {
		// Combine stdout and stderr
		slog.Info("stdout and stderr combined")

		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			line := scanner.Text()

			slog.Info("scanning line", "line", line)

			if strings.Contains(line, "DevTools listening on") {
				parts := strings.Split(line, "DevTools listening on ")
				if len(parts) > 1 {
					wsURLChan <- strings.TrimSpace(parts[1])
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("scanning output failed: %v", err)
		}
	}()

	// Wait for WebSocket URL or timeout
	select {
	case wsURL := <-wsURLChan:
		z.debuggerURL = strings.TrimSpace(wsURL)
		break
	case err := <-errChan:
		return err
	case <-timer.C:

		return fmt.Errorf("timeout waiting for chrome to start")
	}

	conn, _, err := websocket.DefaultDialer.Dial(z.debuggerURL, nil)
	if err != nil {
		return err
	}

	z.wsConn = conn

	go z.sendEventLoop()

	return nil
}

func (z *ZChromeView) Stop() error {
	z.msgChan <- &IPCMessage{
		Method: "Page.close",
		Params: map[string]string{},
	}

	time.Sleep(100 * time.Millisecond)

	if z.cmd != nil {
		if err := z.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %v", err)
		}
	}

	return nil
}

func (z *ZChromeView) NavigateURL(url string) error {
	z.sendGoto(url)
	return nil
}

// helpers

func (z *ZChromeView) sendGoto(url string) {
	z.msgChan <- &IPCMessage{
		Method: "Page.navigate",
		Params: map[string]string{"url": url},
	}
}

func (z *ZChromeView) sendEventLoop() {

	defer z.wsConn.Close()

	for {
		msg := <-z.msgChan

		if msg.ID == 0 {
			msg.ID = z.msgIdCounter
			z.msgIdCounter++
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			slog.Error("failed to marshal message", "err", err)
			continue
		}

		err = z.wsConn.WriteMessage(websocket.TextMessage, msgBytes)
		if err != nil {
			slog.Error("failed to write message", "err", err)
			continue
		}
	}

}

func (z *ZChromeView) generateArgs() []string {
	args := []string{
		"--no-first-run",
		"--remote-debugging-port=0",
		"--new-instance",   // Force new instance
		"--enable-logging", // Help with debugging
	}
	if z.opts.StartUpURL != "" {
		args = append(args, fmt.Sprintf("--app=%s", z.opts.StartUpURL))
	}

	if z.opts.StateMode != StateModeNormal {
		args = append(args, fmt.Sprintf("--user-data-dir=%s", z.profile))
	}

	if z.opts.ExtraArgs != nil {
		args = append(args, z.opts.ExtraArgs...)
	}

	return args
}
