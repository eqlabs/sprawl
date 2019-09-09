package config

import (
	"strings"

	"github.com/prometheus/common/log"
	"github.com/spf13/viper"
)

// Config has an initialized version of spf13/viper
type Config struct {
	v *viper.Viper
}

// ReadConfig opens the configuration file and initializes viper
func (c *Config) ReadConfig(configPath string) {
	// Init viper
	c.v = viper.New()

	// Define where viper tries to get config information
	envPrefix := "sprawl"

	// Set environment variable prefix, automatically transformed to uppercase
	c.v.SetEnvPrefix(envPrefix)

	// Set replacer to env variables, replacing dots with underscores
	c.v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Automatically try to fetch all configs from env
	c.v.AutomaticEnv()

	// Initialize viper with Sprawl-specific options
	c.v.SetConfigName("config")

	// Use toml format for config files
	c.v.SetConfigType("toml")

	// Check for overriding config files
	c.v.AddConfigPath("$GOPATH/src/sprawl/")
	c.v.AddConfigPath("$GOPATH/src/github.com/eqlabs/sprawl/")
	c.v.AddConfigPath(".")

	// Check for user submitted config path
	c.v.AddConfigPath(configPath)

	// Read config file
	if err := c.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Warn("Config file not found, using ENV")
		} else {
			log.Error("Config file invalid!")
		}
	} else {
		log.Info("Config successfully loaded.")
	}
}

// Get is a proxy for viper.Get()
func (c *Config) Get(variable string) interface{} {
	return c.v.Get(variable)
}

// GetString is a proxy for viper.GetString()
func (c *Config) GetString(variable string) string {
	return c.v.GetString(variable)
}

// GetUint is a proxy for viper.GetUint()
func (c *Config) GetUint(variable string) uint {
	return c.v.GetUint(variable)
}

// GetBool is a proxy for viper.GetUint()
func (c *Config) GetBool(variable string) bool {
	return c.v.GetBool(variable)
}
