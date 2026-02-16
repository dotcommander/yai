package cmd

import (
	"fmt"
	goruntime "runtime"
	"runtime/debug"

	"github.com/dotcommander/yai/internal/storage"
)

// BuildInfo is injected by the build pipeline.
type BuildInfo struct {
	Version   string
	CommitSHA string
}

// versionTemplate returns the Cobra version template string including
// commit SHA, Go version, and OS/arch.
func versionTemplate(b BuildInfo) string {
	v := "{{.Name}} {{.Version}}"
	if len(b.CommitSHA) >= storage.SHA1Short {
		v += " (" + b.CommitSHA[:storage.SHA1Short] + ")"
	}
	v += fmt.Sprintf(" %s %s/%s\n", goruntime.Version(), goruntime.GOOS, goruntime.GOARCH)
	return v
}

func normalizeBuildInfo(b BuildInfo) BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		if b.Version == "" {
			b.Version = "unknown"
		}
		return b
	}

	if b.Version == "" {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			b.Version = info.Main.Version
		}
	}

	// Extract VCS info embedded by Go 1.18+.
	var vcsRev, vcsModified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			vcsRev = s.Value
		case "vcs.modified":
			vcsModified = s.Value
		}
	}

	if b.CommitSHA == "" && vcsRev != "" {
		b.CommitSHA = vcsRev
	}

	if b.Version == "" {
		b.Version = "dev"
		if len(vcsRev) >= storage.SHA1Short {
			b.Version += "-" + vcsRev[:storage.SHA1Short]
		}
		if vcsModified == "true" {
			b.Version += "-dirty"
		}
	}

	return b
}
