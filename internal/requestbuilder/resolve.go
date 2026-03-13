package requestbuilder

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
)

// ResolveModel finds the requested API and model in settings.
func ResolveModel(cfg *config.Config) (config.API, config.Model, error) {
	for _, api := range cfg.APIs {
		if api.Name != cfg.API && cfg.API != "" {
			continue
		}
		for name, mod := range api.Models {
			if name == cfg.Model || slices.Contains(mod.Aliases, cfg.Model) {
				cfg.Model = name
				break
			}
		}
		mod, ok := api.Models[cfg.Model]
		if ok {
			mod.Name = cfg.Model
			mod.API = api.Name
			return api, mod, nil
		}
		if cfg.API != "" {
			available := make([]string, 0, len(api.Models))
			for name := range api.Models {
				available = append(available, name)
			}
			slices.Sort(available)
			return config.API{}, config.Model{}, errs.Wrap(
				errs.UserErrorf("Available models are: %s", strings.Join(available, ", ")),
				fmt.Sprintf("The API endpoint %s does not contain the model %s", cfg.API, cfg.Model),
			)
		}
	}

	return config.API{}, config.Model{}, errs.Wrap(
		errs.UserErrorf("Please specify an API endpoint with --api or configure the model in the settings: yai --settings"),
		fmt.Sprintf("Model %s is not in the settings file.", cfg.Model),
	)
}

// IsReasoningModel reports whether the given model name is a reasoning model
// (e.g. o1, o3, o4, gpt-5 series) that does not support temperature/top-p/top-k.
func IsReasoningModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if slash := strings.LastIndex(m, "/"); slash >= 0 && slash < len(m)-1 {
		m = m[slash+1:]
	}

	return strings.HasPrefix(m, "gpt-5") ||
		strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4")
}
