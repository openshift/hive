package hive

import (
	"embed"
	"path/filepath"
	"runtime"
)

//go:embed testdata/*
var testdataFS embed.FS

// FixturePath returns the absolute path to the testdata directory or a
// subdirectory within it. This replaces compat_otp.FixturePath for
// locally-hosted test fixtures.
func FixturePath(elem ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file)}, elem...)...)
}

// Asset reads an embedded testdata file and returns its contents.
// This replaces testdata.Asset from openshift-tests-private.
func Asset(name string) ([]byte, error) {
	return testdataFS.ReadFile(name)
}
