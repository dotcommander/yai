package cmd

import (
	"os"

	"github.com/dotcommander/yai/internal/config"
)

// Execute wires commands and runs Cobra.
func Execute(build BuildInfo, cfg config.Config, cfgErr error) {
	defer maybeWriteMemProfile()

	root := NewRootCmd(build, cfg, cfgErr)
	if err := root.Execute(); err != nil {
		handleError(err)
		os.Exit(1)
	}
}
