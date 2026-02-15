package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/present"
)

func roleNames(cfg *config.Config, prefix string) []string {
	roles := make([]string, 0, len(cfg.Roles))
	for role := range cfg.Roles {
		if prefix != "" && !strings.HasPrefix(role, prefix) {
			continue
		}
		roles = append(roles, role)
	}
	slices.Sort(roles)
	return roles
}

func listRoles(cfg *config.Config) {
	for _, role := range roleNames(cfg, "") {
		s := role
		if role == cfg.Role {
			s = role + present.StdoutStyles().Timeago.Render(" (default)")
		}
		fmt.Println(s)
	}
}
