package testutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

// SamplePackRoot returns the repository's platform-neutral sample fixture.
func SamplePackRoot(t *testing.T) string {
	t.Helper()
	return FixturePath(t, "sample-pack")
}

// FixturePath returns an absolute path under the repository's platform-neutral
// tests/fixtures directory.
func FixturePath(t *testing.T, elements ...string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	parts := append([]string{filepath.Dir(currentFile), "../../../tests/fixtures"}, elements...)
	root, err := filepath.Abs(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
