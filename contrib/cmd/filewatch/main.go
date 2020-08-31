package main

import (
	"fmt"
	"os"

	"github.com/openshift/library-go/pkg/operator/watchdog"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	cmd := watchdog.NewFileWatcherWatchdog()

	err := cmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}
