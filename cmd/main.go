package main

import (
	"os"

	"github.com/ArmaanKatyal/porta/pkg/server"
)

func main() {
	if err := server.Start(); err != nil {
		os.Exit(1)
	}
}
