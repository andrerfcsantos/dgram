package config

import (
	"fmt"
	gap "github.com/muesli/go-app-paths"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	configName = "config"
	configType = "yml"
)

type Config struct {
	AppName  string
	gapScope *gap.Scope
	*viper.Viper
}

func NewConfig(appName string) *Config {
	viperCfg := viper.NewWithOptions(
		viper.EnvKeyReplacer(strings.NewReplacer("_", ".")),
	)

	viperCfg.SetEnvPrefix(appName)
	viperCfg.AutomaticEnv()

	viperCfg.SetConfigName(configName)
	viperCfg.SetConfigType(configType)

	return &Config{
		AppName:  appName,
		Viper:    viperCfg,
		gapScope: gap.NewScope(gap.User, appName),
	}
}

func (c *Config) Read() error {
	paths, err := c.gapScope.LookupConfig(configName + "." + configType)
	if err != nil {
		return fmt.Errorf("getting config path: %w", err)
	}

	for _, path := range slices.Backward(paths) {
		c.SetConfigFile(path)
		err := c.MergeInConfig()
		if err != nil {
			return fmt.Errorf("merging config from '%s': %w", path, err)
		}
	}

	return nil
}

func (c *Config) Write() error {
	paths, err := c.gapScope.LookupConfig(configName + "." + configType)
	if err != nil {
		return fmt.Errorf("getting config path: %w", err)
	}

	if len(paths) == 0 {
		configDirs, err := c.gapScope.ConfigDirs()
		if err != nil {
			return fmt.Errorf("getting config dir alternatives to create a new config file: %w", err)
		}
		configDir := configDirs[0]
		err = os.MkdirAll(configDir, 0750)
		if err != nil {
			return fmt.Errorf("creating config dir: %w", err)
		}

		configPath := filepath.Join(configDir, configName+"."+configType)
		c.SetConfigFile(configPath)
	} else {
		c.SetConfigFile(paths[0])
	}

	err = c.WriteConfig()
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) DataDir() (string, error) {
	dataPaths, err := c.gapScope.DataDirs()
	if err != nil {
		return "", fmt.Errorf("getting data dirs: %w", err)
	}

	if len(dataPaths) == 0 {
		return "", fmt.Errorf("no suitable data dirs found")
	}

	return dataPaths[0], nil
}
