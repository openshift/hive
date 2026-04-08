package resource_test

import (
	"os"
	"testing"

	"github.com/openshift/hive/test/benchutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	benchutil.StopEnvTest()
	os.Exit(code)
}
