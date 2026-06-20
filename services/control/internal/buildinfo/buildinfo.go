// Package buildinfo is the single source of truth for the control plane's
// own version string. Every call site that needs "what version am I?" must
// go through Version() here —never through os.Getenv("APP_VERSION") or a
// private copy of the constant.
//
// Why a package and not a bare global in main? Two reasons:
//   1. We want build_control.sh to inject the real release version via
//      -ldflags "-X .../buildinfo.appVersion=1.2.3" without coupling the
//      linker flag to the main package path.
//   2. The old code had five call sites each reading os.Getenv with a
//      different fallback ("unknown", "1.0.0-beta.1", ""). That produced
//      three different answers from the /api/system/info, /api/system/upgrade/info
//      and /api/control/report endpoints. A single Version() function
//      makes that impossible.
package buildinfo

import (
	"os"
	"strings"
	"sync"
)

// appVersion is the compile-time version of this binary. It is set one of
// two ways, in this order of precedence:
//
//  1. -ldflags "-X github.com/lingcdn/control/internal/buildinfo.appVersion=X.Y.Z"
//     (the release build path; see build_control.sh).
//  2. The fallback string literal below (used only during `go run` / IDE
//     runs / `go test` and for developers who haven't updated the build
//     script yet).
//
// It MUST NOT be overwritten at runtime. The env-based override lives in
// envOverride below and is explicitly a separate variable so operator
// mistakes (e.g. a stale APP_VERSION pinned in /etc/lingcdn/lingcdn.env)
// can never shadow the real binary version —they can only *complement*
// it with operator-visible metadata.
var appVersion = "1.0.24"

// once guards one-time initialization of the resolved version string.
var (
	once     sync.Once
	resolved string
)

// Version returns the version of the running binary. This is the value that
// should appear in:
//   - /api/system/info   (health / UI header)
//   - /api/system/upgrade/info (portal version compare)
//   - /api/control/report (portal system report)
//   - /api/nodes/install script (so new nodes install a matching client)
//   - any log line that prints "我是哪个版本"
//
// It is derived exactly once from the compile-time constant. Environment
// overrides are intentionally IGNORED by this function —see OperatorTag
// if you need a human-readable override channel.
func Version() string {
	once.Do(func() {
		v := strings.TrimSpace(appVersion)
		if v == "" {
			v = "0.0.0-dev"
		}
		resolved = v
	})
	return resolved
}

// OperatorTag returns an optional, operator-supplied tag that complements
// Version(). It exists so operators running multiple control planes can
// tell them apart in the portal report without us re-purposing the
// authoritative version field.
//
// It pulls from APP_VERSION_TAG (not APP_VERSION —that name collided
// with the compile-time version before the refactor and caused the
// "upgraded but portal shows old version" bug).
//
// Callers should prefer Version() for anything that needs to match the
// upgrade pipeline's notion of a version. Use OperatorTag only for
// UI/report display.
func OperatorTag() string {
	return strings.TrimSpace(os.Getenv("APP_VERSION_TAG"))
}

// SetForTest overrides the resolved version for the duration of a test.
// It is the ONLY supported way to change the value after init. Returns a
// function that restores the previous value.
//
// This exists because Version() caches the first answer via sync.Once, so
// tests that need to exercise multiple versions in one process must have
// a supported reset path. Production code MUST NOT call this.
func SetForTest(v string) func() {
	once.Do(func() {}) // ensure once is consumed
	prev := resolved
	resolved = strings.TrimSpace(v)
	if resolved == "" {
		resolved = "0.0.0-dev"
	}
	return func() { resolved = prev }
}
