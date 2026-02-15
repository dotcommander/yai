// Package main provides the yai CLI.
package main

import (
	"github.com/dotcommander/yai/internal/cmd"
	"github.com/dotcommander/yai/internal/config"
)

// Build vars.
var (
	//nolint: gochecknoglobals
	Version = ""
	//nolint: gochecknoglobals
	CommitSHA = ""
)

func main() {
	cfg, cfgErr := config.Ensure()
	cmd.Execute(cmd.BuildInfo{Version: Version, CommitSHA: CommitSHA}, cfg, cfgErr)
}
