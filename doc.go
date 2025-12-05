// Package flux provides reactive configuration synchronization primitives.
//
// The core type is Capacitor, which watches external sources for changes,
// transforms and validates the data, and applies it to application state
// with automatic rollback on failure.
//
// # Capacitor
//
// A Capacitor monitors a source (file, channel, etc.) for changes and
// processes them through a pipeline:
//
//	Source → Transform → Validate → Apply
//
// If any step fails, the previous valid configuration is retained and
// the Capacitor enters a degraded state while continuing to watch for
// valid updates.
//
// # State Machine
//
// Capacitor maintains one of four states:
//
//   - Loading: Initial state, no config yet
//   - Healthy: Valid config applied
//   - Degraded: Last change failed, previous config still active
//   - Empty: Initial load failed, no valid config ever obtained
//
// # Watchers
//
// The Watcher interface abstracts change sources. Built-in implementations:
//
//   - FileWatcher: Watches a file for changes using fsnotify
//   - ChannelWatcher: Wraps an existing channel (useful for testing)
//
// # Example
//
//	type AppConfig struct {
//	    Feature string `yaml:"feature"`
//	    Limit   int    `yaml:"limit"`
//	}
//
//	capacitor := flux.New(
//	    flux.NewFileWatcher("/etc/myapp/config.yaml"),
//	    func(data []byte) (AppConfig, error) {
//	        var cfg AppConfig
//	        return cfg, yaml.Unmarshal(data, &cfg)
//	    },
//	    func(cfg AppConfig) error {
//	        if cfg.Limit < 0 {
//	            return errors.New("limit must be non-negative")
//	        }
//	        return nil
//	    },
//	    func(cfg AppConfig) error {
//	        app.UpdateConfig(cfg)
//	        return nil
//	    },
//	    flux.WithDebounce(100*time.Millisecond),
//	)
//
//	if err := capacitor.Start(ctx); err != nil {
//	    log.Printf("initial config failed: %v", err)
//	}
package flux
