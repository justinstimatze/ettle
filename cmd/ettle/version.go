package main

import "runtime/debug"

// version is overridden at release via -ldflags "-X main.version=$(git describe ...)".
// The git tag is the single source of truth; this is just the fallback default.
var version = "dev"

// buildVersion resolves the version string, preferring the ldflags-baked value,
// then the module version (go install …@vX.Y.Z), then the local VCS revision,
// then "dev".
func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if dirty {
			rev += "-dirty"
		}
		return rev
	}
	return version
}
