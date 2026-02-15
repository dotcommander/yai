package cmd

import "github.com/dotcommander/yai/internal/config"

func isNoArgs(cfg *config.Config) bool {
	return cfg.Prefix == "" &&
		cfg.Show == "" &&
		!cfg.ShowLast &&
		len(cfg.Delete) == 0 &&
		cfg.DeleteOlderThan == 0 &&
		!cfg.ShowHelp &&
		!cfg.List &&
		!cfg.ListRoles &&
		!cfg.MCPList &&
		!cfg.MCPListTools &&
		!cfg.Dirs &&
		!cfg.EditSettings &&
		!cfg.ResetSettings
}
