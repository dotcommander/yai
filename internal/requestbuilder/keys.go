package requestbuilder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/caarlos0/go-shellwords"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
)

func ensureKey(ctx context.Context, api config.API, defaultEnv, docsURL string) (string, error) {
	key, err := resolveConfiguredKey(ctx, api)
	if err != nil {
		return "", err
	}
	if key == "" {
		key = os.Getenv(defaultEnv)
	}
	if key != "" {
		return key, nil
	}
	return "", errs.Wrap(
		errs.UserErrorf("You can grab one at %s", docsURL),
		fmt.Sprintf("%s required; set %s or update yai.yml through yai --settings.", defaultEnv, defaultEnv),
	)
}

func optionalKey(ctx context.Context, api config.API) (string, error) {
	return resolveConfiguredKey(ctx, api)
}

func resolveConfiguredKey(ctx context.Context, api config.API) (string, error) {
	key := api.APIKey
	if key == "" && api.APIKeyEnv != "" && api.APIKeyCmd == "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	if key == "" && api.APIKeyCmd != "" {
		resolved, err := keyFromCommand(ctx, api.APIKeyCmd)
		if err != nil {
			return "", err
		}
		key = resolved
	}
	return key, nil
}

func keyFromCommand(ctx context.Context, cmd string) (string, error) {
	args, err := shellwords.Parse(cmd)
	if err != nil {
		return "", errs.Wrap(err, "Failed to parse api-key-cmd")
	}
	out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput() //nolint:gosec // G204: api-key-cmd is user-configured in yai.yml, intentional shell command
	if err != nil {
		return "", errs.Wrap(err, "Cannot exec api-key-cmd")
	}
	return strings.TrimSpace(string(out)), nil
}
