package cmd

import (
	"fmt"
	"os"

	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"
)

func newManCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "man",
		Short:                 "Generates manpages",
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		Hidden:                true,
		Args:                  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			manPage, err := mcobra.NewManPage(1, root)
			if err != nil {
				return fmt.Errorf("build man page: %w", err)
			}
			_, err = fmt.Fprint(os.Stdout, manPage.Build(roff.NewDocument()))
			if err != nil {
				return fmt.Errorf("write man page: %w", err)
			}
			return nil
		},
	}
}
