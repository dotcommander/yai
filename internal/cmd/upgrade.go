package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

const installPkg = "github.com/dotcommander/yai@latest"

func newUpgradeCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade yai to the latest version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !rt.cfg.Quiet {
				fmt.Fprintf(os.Stderr, "Current version: %s\n", rt.build.Version)
				fmt.Fprintf(os.Stderr, "Upgrading via go install %s ...\n", installPkg)
			}

			gobin, err := exec.LookPath("go")
			if err != nil {
				return fmt.Errorf("go not found in PATH: %w", err)
			}

			install := exec.Command(gobin, "install", installPkg)
			install.Stdout = os.Stdout
			install.Stderr = os.Stderr
			if err := install.Run(); err != nil {
				return fmt.Errorf("go install failed: %w", err)
			}

			if !rt.cfg.Quiet {
				fmt.Fprintln(os.Stderr, "Upgrade complete.")
			}
			return nil
		},
	}
}
