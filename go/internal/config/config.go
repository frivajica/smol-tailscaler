// Package config carries the runtime settings resolved from build-time
// ldflags values, CLI overrides, and interactive prompts.
package config

// Config holds the settings shared by every setup step.
type Config struct {
	TargetUser   string
	UserPassword string
	TsAuthKey    string
}
