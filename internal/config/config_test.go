package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFormatText(t *testing.T) {
	t.Run("old format text", func(t *testing.T) {
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte("format-text: as markdown"), &cfg))
		require.Equal(t, FormatText(map[string]string{"markdown": "as markdown"}), cfg.FormatText)
	})

	t.Run("new format text", func(t *testing.T) {
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte("format-text:\n  markdown: as markdown\n  json: as json"), &cfg))
		require.Equal(t, FormatText(map[string]string{"markdown": "as markdown", "json": "as json"}), cfg.FormatText)
	})
}

func TestMergeRolesFromDir(t *testing.T) {
	t.Run("loads text role files as file references", func(t *testing.T) {
		root := t.TempDir()
		rolesDir := filepath.Join(root, "roles")
		require.NoError(t, os.MkdirAll(rolesDir, 0o700))
		file := filepath.Join(rolesDir, "shell.md")
		require.NoError(t, os.WriteFile(file, []byte("you are a shell expert"), 0o600))

		cfg := Config{Runtime: Runtime{SettingsPath: filepath.Join(root, "yai.yml")}}
		require.NoError(t, MergeRolesFromDir(&cfg))
		require.Equal(t, []string{"file://" + file}, cfg.Roles["shell"])
	})

	t.Run("loads markdown role definitions as file references", func(t *testing.T) {
		root := t.TempDir()
		rolesDir := filepath.Join(root, "roles")
		require.NoError(t, os.MkdirAll(rolesDir, 0o700))
		reviewer := filepath.Join(rolesDir, "reviewer.md")
		single := filepath.Join(rolesDir, "single.md")
		require.NoError(t, os.WriteFile(reviewer, []byte("be concise\nbe precise\n"), 0o600))
		require.NoError(t, os.WriteFile(single, []byte("be calm"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(rolesDir, "ignore.yml"), []byte("- ignored\n"), 0o600))

		cfg := Config{Runtime: Runtime{SettingsPath: filepath.Join(root, "yai.yml")}}
		require.NoError(t, MergeRolesFromDir(&cfg))
		require.Equal(t, []string{"file://" + reviewer}, cfg.Roles["reviewer"])
		require.Equal(t, []string{"file://" + single}, cfg.Roles["single"])
		require.Nil(t, cfg.Roles["ignore"]) // .yml files are skipped
	})

	t.Run("config roles override directory roles", func(t *testing.T) {
		root := t.TempDir()
		rolesDir := filepath.Join(root, "roles")
		require.NoError(t, os.MkdirAll(rolesDir, 0o700))
		shellPath := filepath.Join(rolesDir, "shell.md")
		newRolePath := filepath.Join(rolesDir, "new-role.md")
		require.NoError(t, os.WriteFile(shellPath, []byte("from dir\n"), 0o600))
		require.NoError(t, os.WriteFile(newRolePath, []byte("only in dir\n"), 0o600))

		cfg := Config{
			Settings: Settings{Roles: map[string][]string{"shell": {"from config"}}},
			Runtime:  Runtime{SettingsPath: filepath.Join(root, "yai.yml")},
		}
		require.NoError(t, MergeRolesFromDir(&cfg))
		require.Equal(t, []string{"from config"}, cfg.Roles["shell"])
		require.Equal(t, []string{"file://" + newRolePath}, cfg.Roles["new-role"])
	})

	t.Run("loads nested roles recursively with path-based names", func(t *testing.T) {
		root := t.TempDir()
		rolesDir := filepath.Join(root, "roles")
		nested := filepath.Join(rolesDir, "philosophy", "greek")
		require.NoError(t, os.MkdirAll(nested, 0o700))

		stoicPath := filepath.Join(nested, "stoic.md")
		require.NoError(t, os.WriteFile(stoicPath, []byte("keep perspective\n"), 0o600))
		helpersPath := filepath.Join(rolesDir, "helpers", "shell.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(helpersPath), 0o700))
		require.NoError(t, os.WriteFile(helpersPath, []byte("you are a shell expert"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(rolesDir, "philosophy", "greek", "ignore.yml"), []byte("- ignored\n"), 0o600))

		cfg := Config{Runtime: Runtime{SettingsPath: filepath.Join(root, "yai.yml")}}
		require.NoError(t, MergeRolesFromDir(&cfg))
		require.Equal(t, []string{"file://" + stoicPath}, cfg.Roles["philosophy/greek/stoic"])
		require.Equal(t, []string{"file://" + helpersPath}, cfg.Roles["helpers/shell"])
		require.Nil(t, cfg.Roles["philosophy/greek/ignore"]) // .yml files are skipped
	})

	t.Run("skips yaml files with complex manifest structure", func(t *testing.T) {
		root := t.TempDir()
		rolesDir := filepath.Join(root, "roles")
		require.NoError(t, os.MkdirAll(rolesDir, 0o700))

		// Create a complex YAML manifest that should be skipped
		manifestYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
data:
  config.json: |
    {"key": "value"}
`
		require.NoError(t, os.WriteFile(filepath.Join(rolesDir, "manifest.yaml"), []byte(manifestYAML), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(rolesDir, "config.yml"), []byte("key: value\n"), 0o600))

		// Also create a valid .md role
		mdPath := filepath.Join(rolesDir, "valid.md")
		require.NoError(t, os.WriteFile(mdPath, []byte("valid role content"), 0o600))

		cfg := Config{Runtime: Runtime{SettingsPath: filepath.Join(root, "yai.yml")}}
		require.NoError(t, MergeRolesFromDir(&cfg))

		// Only .md file should be loaded
		require.Equal(t, []string{"file://" + mdPath}, cfg.Roles["valid"])
		require.Nil(t, cfg.Roles["manifest"])
		require.Nil(t, cfg.Roles["config"])
	})
}

func TestInstallStarterRoles(t *testing.T) {
	t.Run("creates tldr.md role file", func(t *testing.T) {
		configDir := t.TempDir()
		rolesDir := filepath.Join(configDir, "roles")

		installStarterRoles(configDir)

		tldrPath := filepath.Join(rolesDir, "tldr.md")
		require.FileExists(t, tldrPath)

		content, err := os.ReadFile(tldrPath)
		require.NoError(t, err)
		require.NotEmpty(t, content)
		require.Contains(t, string(content), "concise")
	})

	t.Run("does not overwrite existing tldr.md", func(t *testing.T) {
		configDir := t.TempDir()
		rolesDir := filepath.Join(configDir, "roles")
		require.NoError(t, os.MkdirAll(rolesDir, 0o700))

		tldrPath := filepath.Join(rolesDir, "tldr.md")
		existingContent := "custom tldr content"
		require.NoError(t, os.WriteFile(tldrPath, []byte(existingContent), 0o600))

		installStarterRoles(configDir)

		content, err := os.ReadFile(tldrPath)
		require.NoError(t, err)
		require.Equal(t, existingContent, string(content))
	})
}

func TestCreateConfigFileInstallsStarterRoles(t *testing.T) {
	t.Run("createConfigFile installs starter roles", func(t *testing.T) {
		configDir := t.TempDir()
		configPath := filepath.Join(configDir, "yai.yml")

		err := createConfigFile(configPath)
		require.NoError(t, err)

		tldrPath := filepath.Join(configDir, "roles", "tldr.md")
		require.FileExists(t, tldrPath)

		content, err := os.ReadFile(tldrPath)
		require.NoError(t, err)
		require.NotEmpty(t, content)
	})
}
