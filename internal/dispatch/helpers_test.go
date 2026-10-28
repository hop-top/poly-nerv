package dispatch

import "hop.top/nerv/internal/config"

// compileCfg builds a CachedConfig from variadic HookEntry args.
func compileCfg(hooks ...config.HookEntry) *config.CachedConfig {
	return config.Compile(&config.HooksConfig{Hooks: hooks})
}
