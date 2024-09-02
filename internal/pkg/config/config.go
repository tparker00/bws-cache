package config

import (
	"runtime/debug"
	"strings"
	"time"

	"bws-cache/internal/pkg/client"

	"github.com/spf13/viper"
)

type Config struct {
	Port          int           `mapstructure:"port"`
	LogLevel      string        `mapstructure:"log_level"`
	OrgID         string        `mapstructure:"org_id"`
	SecretTTL     time.Duration `mapstructure:"secret_ttl"`
	WebTTL        time.Duration `mapstructure:"web_ttl"`
	RefreshKeyMap bool          `mapstructure:"refresh_keymap_on_miss"`
	Connection    client.Bitwarden
}

var Commit = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}

	return ""
}()

func LoadConfig(config *Config) {
	v := viper.New()
	v.SetEnvPrefix("bws_cache")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("port", 8080)
	v.SetDefault("log_level", "info")
	v.SetDefault("org_id", "")
	v.SetDefault("secret_ttl", "15m")
	v.SetDefault("web_ttl", "5s")
	v.SetDefault("refresh_keymap_on_miss", true)
	v.AutomaticEnv()

	v.Unmarshal(config)
}
