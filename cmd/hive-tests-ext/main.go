package main

import (
	"fmt"
	"os"

	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	"github.com/spf13/cobra"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	e "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"

	// If using ginkgo, import your tests here
	_ "github.com/openshift/hive/test/e2e"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()
	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)

	// These flags are used to pull in the default values to test context - required
	// so tests run correctly, even if the underlying flags aren't used.
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)

	// Create our registry of openshift-tests extensions
	extensionRegistry := e.NewRegistry()
	hiveTestsExtension := e.NewExtension("openshift", "payload", "hive")
	extensionRegistry.Register(hiveTestsExtension)

	// Carve up the hive tests into our openshift suites...
	hiveTestsExtension.AddSuite(e.Suite{
		Name: "hive/conformance/parallel",
		Parents: []string{
			"openshift/conformance/parallel",
		},
		Qualifiers: []string{`!labels.exists(l, l == "Serial") && labels.exists(l, l == "Conformance")`},
	})

	hiveTestsExtension.AddSuite(e.Suite{
		Name: "hive/conformance/serial",
		Parents: []string{
			"openshift/conformance/serial",
		},
		Qualifiers: []string{`labels.exists(l, l == "Serial") && labels.exists(l, l == "Conformance")`},
	})

	// Build our specs from ginkgo
	specs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		panic(err)
	}

	// Initialization for kube ginkgo test framework needs to run before all tests execute
	specs.AddBeforeAll(func() {
		if err := initializeTestFramework(os.Getenv("TEST_PROVIDER")); err != nil {
			panic(err)
		}
	})

	// Let's scan for tests with a platform label and create the rule for them such as [platform:aws]
	foundPlatforms := make(map[string]string)
	for _, test := range specs.Select(extensiontests.NameContains("[platform:")).Names() {
		re := regexp.MustCompile(`\[platform:[a-z]*]`)
		match := re.FindStringSubmatch(test)
		for _, platformDef := range match {
			if _, ok := foundPlatforms[platformDef]; !ok {
				platform := platformDef[strings.Index(platformDef, ":")+1 : len(platformDef)-1]
				foundPlatforms[platformDef] = platform
				specs.Select(extensiontests.NameContains(platformDef)).
					Include(extensiontests.PlatformEquals(platform))
			}
		}

	}

	hiveTestsExtension.AddSpecs(specs)

	// Cobra stuff
	root := &cobra.Command{
		Long: "Hive tests extension for OpenShift",
	}

	root.AddCommand(cmd.DefaultExtensionCommands(extensionRegistry)...)

	if err := func() error {
		return root.Execute()
	}(); err != nil {
		os.Exit(1)
	}
}