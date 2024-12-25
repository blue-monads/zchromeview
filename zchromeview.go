package zchromeview

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/gorilla/websocket"
	"github.com/k0kubun/pp"
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
	sessionId    string
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
		profile = path.Join(dir, ".zchromeview")

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
		msgIdCounter: 1,
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

	cmd.Stdout = os.Stdout

	// Capture stdout and stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	slog.Info("before start")

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start chrome: %v", err)
	}

	buf := bufio.NewReader(stderr)
	for {
		line, _, err := buf.ReadLine()
		if err != nil {
			return fmt.Errorf("failed to read stdout: %v", err)
		}

		sline := string(line)

		slog.Info("stdout", "line", sline)

		if strings.Contains(sline, "DevTools listening on") {
			parts := strings.Split(sline, "DevTools listening on ")
			if len(parts) > 1 {
				z.debuggerURL = strings.TrimSpace(parts[1])
				break
			}
		}

		slog.Info("stdout", "line", string(line))
	}

	pp.Println("@debug/url", z.debuggerURL)

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

func (z *ZChromeView) receiveEventLoop() {

	for {

		msg := &IPCMessage{}

		err := z.wsConn.ReadJSON(msg)
		if err != nil {
			pp.Println("read error", err)
			return
		}

		pp.Println("@GOT", msg)

		if msg.Method == "Target.attachedToTarget" {
			params := msg.Params.(map[string]interface{})
			z.sessionId = params["sessionId"].(string)
			break
		}

	}

}

func (z *ZChromeView) msgId() int64 {
	z.msgIdCounter = z.msgIdCounter + 1
	return z.msgIdCounter
}

func (z *ZChromeView) sendEventLoop() {

	defer z.wsConn.Close()

	err := z.wsConn.WriteJSON(&IPCMessage{
		ID:     z.msgId(),
		Method: "Target.getTargets",
	})

	if err != nil {
		slog.Error("failed to write message", "err", err)
		return
	}

	// Read response
	var resp struct {
		ID     int `json:"id"`
		Result struct {
			TargetInfos []TargetInfo `json:"targetInfos"`
		} `json:"result"`
	}

	err = z.wsConn.ReadJSON(&resp)
	if err != nil {
		slog.Error("failed to read message", "err", err)
		return
	}

	pp.Println("@resp", resp)

	// Find the first page target
	var pageTarget *TargetInfo
	for _, target := range resp.Result.TargetInfos {
		if target.Type == "page" {
			pageTarget = &target
			break
		}
	}

	if pageTarget == nil {
		panic("no page target found")
	}

	z.wsConn.WriteJSON(&IPCMessage{
		ID:     z.msgId(),
		Method: "Target.activateTarget",
		Params: map[string]interface{}{
			"targetId": pageTarget.TargetId,
		},
	})

	// attachToTarget

	z.wsConn.WriteJSON(&IPCMessage{
		ID:     z.msgId(),
		Method: "Target.attachToTarget",
		Params: map[string]interface{}{
			"targetId": pageTarget.TargetId,
			"flatten":  true,
		},
	})

	pp.Println("@pageTarget", pageTarget)

	go z.receiveEventLoop()

	for {
		msg := <-z.msgChan

		msg.ID = z.msgId()
		msg.SessionId = z.sessionId

		pp.Println("@sendEventLoop", msg)

		err := z.wsConn.WriteJSON(msg)
		if err != nil {
			slog.Error("failed to write message", "err", err)
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

func (z *ZChromeView) Wait() error {
	if z.cmd == nil {
		return fmt.Errorf("command not started")
	}

	return z.cmd.Wait()
}
