// Package buildinfo exposes release metadata injected by the build pipeline.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// These variables are replaced with -ldflags for release builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Info is the stable machine-readable build identity.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Current returns the running binary's build identity.
func Current() Info {
	info := Info{Version: Version, Commit: Commit, Date: Date}
	if info.Version != "dev" {
		return info
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	if build.Main.Version != "" && build.Main.Version != "(devel)" {
		info.Version = strings.TrimPrefix(build.Main.Version, "v")
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == "unknown" && setting.Value != "" {
				info.Commit = setting.Value
			}
		case "vcs.time":
			if info.Date == "unknown" && setting.Value != "" {
				info.Date = setting.Value
			}
		}
	}
	return info
}
