package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/contrib/pkg/watchdog"
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
