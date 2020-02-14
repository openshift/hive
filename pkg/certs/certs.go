package certs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/library-go/pkg/controller/fileobserver"
)

var (
	filenames = []string{"tls.crt", "tls.key"}
)

// TerminateOnCertChanges terminates the process when either tls.crt or tls.key in the specified certs directory change.
func TerminateOnCertChanges(certsDir string) {
	obs, err := fileobserver.NewObserver(10 * time.Second)
	if err != nil {
		log.WithError(err).Fatal("could not set up file observer to check for cert changes")
	}
	files := make(map[string][]byte, len(filenames))
	fullFilenames := make([]string, len(filenames))
	for i, fn := range filenames {
		fullFilename := filepath.Join(certsDir, fn)
		fullFilenames[i] = fullFilename
		fileBytes, err := ioutil.ReadFile(fullFilename)
		if err != nil {
			log.WithError(err).WithField("filename", fullFilename).Fatal("Unable to read initial content of cert file")
		}
		files[fullFilename] = fileBytes
	}
	obs.AddReactor(func(filename string, action fileobserver.ActionType) error {
		log.WithField("filename", filename).Info("exiting because cert changed")
		// TODO: Exit more cleanly, allowing for cleanup by closing contexts.
		os.Exit(0)
		return nil
	}, files, fullFilenames...)

	stop := make(chan struct{})
	go func() {
		log.WithField("files", fullFilenames).Info("running file observer")
		obs.Run(stop)
		log.Fatal("file observer stopped")
	}()
}
