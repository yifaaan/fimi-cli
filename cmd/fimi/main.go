package main

import (
	"log"
	"os"

	"fimi-cli/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		log.Printf("fimi: %v", err)
		os.Exit(1)
	}
}
