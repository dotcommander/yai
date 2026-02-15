package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/x/editor"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/spf13/cobra"
)

func newConfigCmd(rt *runtime) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage settings",
		RunE: func(_ *cobra.Command, _ []string) error {
			// Allow opening settings even when config parsing failed.
			return editSettings(&rt.cfg)
		},
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open settings in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return editSettings(&rt.cfg)
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Reset settings to defaults",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Allow reset even when config parsing failed.
			return resetSettings(&rt.cfg)
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "dirs",
		Short: "Print config and cache directories",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			printDirs(&rt.cfg, args)
			return nil
		},
	})

	return configCmd
}

func editSettings(cfg *config.Config) error {
	if err := config.WriteConfigFile(cfg.SettingsPath); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	appName := filepath.Base(os.Args[0])
	c, err := editor.Cmd(appName, cfg.SettingsPath)
	if err != nil {
		return errs.Error{Err: err, Reason: "Could not edit your settings file."}
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return errs.Error{Err: err, Reason: fmt.Sprintf(
			"Missing %s.",
			present.StderrStyles().InlineCode.Render("$EDITOR"),
		)}
	}

	if !cfg.Quiet {
		fmt.Fprintln(os.Stderr, "Wrote config file to:", cfg.SettingsPath)
	}
	return nil
}

func resetSettings(cfg *config.Config) error {
	_, err := os.Stat(cfg.SettingsPath)
	if err != nil {
		return errs.Error{Err: err, Reason: "Couldn't read config file."}
	}
	inputFile, err := os.Open(cfg.SettingsPath)
	if err != nil {
		return errs.Error{Err: err, Reason: "Couldn't open config file."}
	}
	defer inputFile.Close() //nolint:errcheck

	outputFile, err := os.Create(cfg.SettingsPath + ".bak")
	if err != nil {
		return errs.Error{Err: err, Reason: "Couldn't backup config file."}
	}
	defer outputFile.Close() //nolint:errcheck

	if _, err := io.Copy(outputFile, inputFile); err != nil {
		return errs.Error{Err: err, Reason: "Couldn't write config file."}
	}
	if err := os.Remove(cfg.SettingsPath); err != nil {
		return errs.Error{Err: err, Reason: "Couldn't remove config file."}
	}
	if err := config.WriteConfigFile(cfg.SettingsPath); err != nil {
		return errs.Error{Err: err, Reason: "Couldn't write new config file."}
	}

	if !cfg.Quiet {
		fmt.Fprintln(os.Stderr, "\nSettings restored to defaults!")
		fmt.Fprintf(
			os.Stderr,
			"\n  %s %s\n\n",
			present.StderrStyles().Comment.Render("Your old settings have been saved to:"),
			present.StderrStyles().Link.Render(cfg.SettingsPath+".bak"),
		)
	}
	return nil
}

func printDirs(cfg *config.Config, args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "config":
			fmt.Println(filepath.Dir(cfg.SettingsPath))
			return
		case "cache":
			fmt.Println(cfg.CachePath)
			return
		}
	}

	fmt.Printf("Configuration: %s\n", filepath.Dir(cfg.SettingsPath))
	//nolint:mnd
	fmt.Printf("%*sCache: %s\n", 8, " ", cfg.CachePath)
}
