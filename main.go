package main

import (
	"log"
	"os"

	"mygoify/internal/app"
)

func main() {
	run := app.Run
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "agent":
			// The agent, which stays while the window is closed: no window.
			run = app.RunAgent
		case "panel":
			// The mini player, as its icon is clicked.
			run = app.RunPanel
		}
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
