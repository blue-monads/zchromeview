package main

import (
	"log/slog"

	"github.com/blue-monads/zchromeview"
)

func main() {
	slog.Info("Hello, ZChromeView!")

	view := zchromeview.New(zchromeview.Options{
		Name:       "spotify",
		StateMode:  zchromeview.StateModeIsolated,
		StartUpURL: "https://open.spotify.com/",
	})

	if err := view.Start(); err != nil {
		slog.Error("failed to start view:", "err", err)
		return
	}

	defer view.Stop()

	slog.Info("view started")

}
