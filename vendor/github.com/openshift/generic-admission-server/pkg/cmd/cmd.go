package cmd

import (
	"os"
	"runtime"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	"github.com/openshift/generic-admission-server/pkg/apiserver"
	"github.com/openshift/generic-admission-server/pkg/cmd/server"
)

// AdmissionHook is what callers provide, in the mutating, the validating variant or implementing even both interfaces.
// We define it here to limit how much of the import tree callers have to deal with for this plugin. This means that
// callers need to match levels of apimachinery, api, client-go, and apiserver.
type AdmissionHook apiserver.AdmissionHook
type ValidatingAdmissionHook apiserver.ValidatingAdmissionHook
type MutatingAdmissionHook apiserver.MutatingAdmissionHook

func RunAdmissionServer(admissionHooks ...AdmissionHook) {
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	stopCh := genericapiserver.SetupSignalHandler()

	// done to avoid cannot use admissionHooks (type []AdmissionHook) as type []apiserver.AdmissionHook in argument to "github.com/openshift/kubernetes-namespace-reservation/pkg/genericadmissionserver/cmd/server".NewCommandStartAdmissionServer
	var castSlice []apiserver.AdmissionHook
	for i := range admissionHooks {
		castSlice = append(castSlice, admissionHooks[i])
	}

	code := cli.Run(server.NewCommandStartAdmissionServer(os.Stdout, os.Stderr, stopCh, castSlice...))
	os.Exit(code)
}
