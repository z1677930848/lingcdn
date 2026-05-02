package server

import (
	"testing"

	"github.com/lingcdn/control/internal/config"
)

func TestStaticLicenseIndexDisabled(t *testing.T) {
	srv := &Servers{
		cfg: config.Config{
			LicenseMode:           "online",
			LicenseStaticIndexURL: "file:///tmp/licenses.json",
		},
	}
	if got := srv.staticLicenseIndexURL(); got != "" {
		t.Fatalf("staticLicenseIndexURL=%q want empty", got)
	}
}
