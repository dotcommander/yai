package cmd

import (
	"runtime/debug"
)

// BuildInfo is injected by the build pipeline.
type BuildInfo struct {
	Version   string
	CommitSHA string
}

func normalizeBuildInfo(b BuildInfo) BuildInfo {
	if b.Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			b.Version = info.Main.Version
		} else {
			b.Version = "unknown (built from source)"
		}
	}
	return b
}
