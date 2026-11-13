//go:build !dev

package webspa

import (
	"io/fs"
	"testing"
)

func TestSPAEmbedsUnderscoreRouteChunks(t *testing.T) {
	matches, err := fs.Glob(SPA, "build/spa/assets/_index-*.js")
	if err != nil {
		t.Fatalf("glob SPA chunks: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("SPA assets beginning with underscore were not embedded")
	}
}
