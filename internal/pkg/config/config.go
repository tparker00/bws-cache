package config

import (
	_ "embed"
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

//go:generate sh -c "printf %s $(git rev-parse HEAD) > commit.txt"
//go:embed commit.txt
var Commit string

//go:generate sh -c "printf %s $(git describe --tags) > commit.txt"
//go:embed version.txt
var Version string

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
