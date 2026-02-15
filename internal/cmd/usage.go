package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotcommander/yai/internal/present"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

func useLine() string {
	appName := filepath.Base(os.Args[0])

	if present.StdoutRenderer().ColorProfile() == termenv.TrueColor {
		appName = present.MakeGradientText(present.StdoutStyles().AppName, appName)
	}

	return fmt.Sprintf(
		"%s %s",
		appName,
		present.StdoutStyles().CliArgs.Render("[OPTIONS] [PREFIX TERM]"),
	)
}

func usageFunc(cmd *cobra.Command) error {
	fmt.Printf(
		"Usage:\n  %s\n\n",
		useLine(),
	)
	fmt.Println("Options:")
	cmd.Flags().VisitAll(func(f *flag.Flag) {
		if f.Hidden {
			return
		}
		if f.Shorthand == "" {
			fmt.Printf(
				"  %-44s %s\n",
				present.StdoutStyles().Flag.Render("--"+f.Name),
				present.StdoutStyles().FlagDesc.Render(f.Usage),
			)
		} else {
			fmt.Printf(
				"  %s%s %-40s %s\n",
				present.StdoutStyles().Flag.Render("-"+f.Shorthand),
				present.StdoutStyles().FlagComma,
				present.StdoutStyles().Flag.Render("--"+f.Name),
				present.StdoutStyles().FlagDesc.Render(f.Usage),
			)
		}
	})
	if cmd.HasExample() {
		fmt.Printf(
			"\nExample:\n  %s\n  %s\n",
			present.StdoutStyles().Comment.Render("# "+cmd.Example),
			cheapHighlighting(present.StdoutStyles(), examples[cmd.Example]),
		)
	}

	return nil
}
