package main

import (
	"log"
	"os"

	"mygoify/internal/app"
)

func main() {
	run := app.Run
	if len(os.Args) > 1 && os.Args[1] == "agent" {
		// The agent, which stays while the window is closed: no window.
		run = app.RunAgent
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
