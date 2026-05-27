package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	e "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"

	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	// Import test packages to register Ginkgo tests
	_ "github.com/openshift/hive/test/ote/hive"
)

func init() {
	// The OpenShift Ginkgo fork (TRT-2539) introduced ForwardingOutputInterceptor
	// which dups fd 1 and forwards all captured output back to stdout. This breaks
	// OTE's subprocess JSON protocol — non-JSON text appears before the result,
	// causing json.Unmarshal to fail with "Deserializaion Error".
	flag.Set("ginkgo.output-interceptor-mode", "none")
}

var platformFileSelectors = map[string]string{
	"hive_aws.go":     "aws",
	"hive_azure.go":   "azure",
	"hive_gcp.go":     "gcp",
	"hive_vsphere.go": "vsphere",
}

func main() {
	registry := e.NewRegistry()

	ext := e.NewExtension("openshift", "optional", "hive")

	ext.AddSuite(e.Suite{
		Name: "openshift/hive",
	})

	selectFns := []et.SelectFunction{hiveTestsOnly()}
	if fn := tagTests(); fn != nil {
		selectFns = append(selectFns, fn)
	}
	if shard := shardTests(); shard != nil {
		selectFns = append(selectFns, shard)
	}
	if limit := limitTests(); limit != nil {
		selectFns = append(selectFns, limit)
	}

	specs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite(selectFns...)
	if err != nil {
		panic(fmt.Sprintf("couldn't build extension test specs from ginkgo: %+v", err.Error()))
	}

	specs.AddBeforeAll(func() {
		exutil.WithCleanup(func() {})
		if err := compat_otp.InitTest(false); err != nil {
			panic(fmt.Sprintf("failed to initialize test framework: %v", err))
		}
		e2e.AfterReadingAllFlags(compat_otp.TestContext)
	})

	applyEnvironmentSelectors(specs)

	specs.Walk(func(spec *et.ExtensionTestSpec) {
		if strings.Contains(spec.Name, "Longduration") {
			spec.Labels.Insert("Longduration")
		}
		spec.Lifecycle = et.LifecycleInforming
	})

	ext.AddSpecs(specs)
	registry.Register(ext)

	root := &cobra.Command{
		Long: "OpenShift Hive OTE Extension",
	}

	root.AddCommand(cmd.DefaultExtensionCommands(registry)...)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// testPlatform returns the platform derived from the test's source file.
// Tests with explicit platform tags in their name (e.g. [Hive/GCP])
// override the file-based platform.
func testPlatform(spec *et.ExtensionTestSpec) string {
	explicitTagToPlatform := map[string]string{
		"[Hive/AWS]":     "aws",
		"[Hive/Azure]":   "azure",
		"[Hive/GCP]":     "gcp",
		"[Hive/vSphere]": "vsphere",
	}
	for tag, platform := range explicitTagToPlatform {
		if strings.Contains(spec.Name, tag) {
			return platform
		}
	}
	for _, cl := range spec.CodeLocations {
		for file, platform := range platformFileSelectors {
			if strings.Contains(cl, file) {
				return platform
			}
		}
	}
	return ""
}

func hiveTestsOnly() et.SelectFunction {
	targetPlatform := os.Getenv("PLATFORM")
	return func(spec *et.ExtensionTestSpec) bool {
		if !strings.Contains(spec.Name, "[sig-hive]") {
			return false
		}
		if targetPlatform != "" {
			p := testPlatform(spec)
			if p != "" && p != targetPlatform {
				return false
			}
		}
		return true
	}
}

func applyEnvironmentSelectors(specs et.ExtensionTestSpecs) {
	specs.Walk(func(spec *et.ExtensionTestSpec) {
		p := testPlatform(spec)
		if p != "" {
			spec.Include(et.PlatformEquals(p))
		}
	})
}

func limitTests() et.SelectFunction {
	limitStr := os.Getenv("TEST_LIMIT")
	if limitStr == "" {
		return nil
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return nil
	}
	count := 0
	return func(spec *et.ExtensionTestSpec) bool {
		if count >= limit {
			return false
		}
		count++
		return true
	}
}

func shardTests() et.SelectFunction {
	indexStr := os.Getenv("TEST_SHARD_INDEX")
	totalStr := os.Getenv("TEST_TOTAL_SHARDS")
	if indexStr == "" || totalStr == "" {
		return nil
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return nil
	}
	total, err := strconv.Atoi(totalStr)
	if err != nil || total <= 0 {
		return nil
	}
	if index < 0 || index >= total {
		return nil
	}
	position := 0
	return func(spec *et.ExtensionTestSpec) bool {
		mine := position%total == index
		position++
		return mine
	}
}

func tagTests() et.SelectFunction {
	tag := os.Getenv("TEST_TAG")
	if tag == "" {
		return nil
	}
	return func(spec *et.ExtensionTestSpec) bool {
		return strings.Contains(spec.Name, tag)
	}
}
