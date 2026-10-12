package server

import (
	"strings"
	"testing"
)

// A typo in the mode must stop startup before anything touches the database.
func TestBootstrapRejectsUnknownSiteMode(t *testing.T) {
	t.Setenv("ORDRYN_BOOTSTRAP_SITE_MODE", "single")
	err := RunBootstrap()
	if err == nil || !strings.Contains(err.Error(), "ORDRYN_BOOTSTRAP_SITE_MODE") {
		t.Fatalf("RunBootstrap() err = %v, want ORDRYN_BOOTSTRAP_SITE_MODE error", err)
	}
}
