package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/spf13/viper"
)

func initConfig() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	home, homedirErr := os.UserHomeDir()
	if homedirErr != nil {
		logger.Fatal().AnErr("error", homedirErr).Msg("failed to detect home dir")
	}
	configDir := filepath.Join(home, ".orion")
	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yml")

	if err := os.MkdirAll(configDir, readWrite); err != nil {
		logger.Fatal().AnErr("error", err).Msg("failed to create config dir")
	}
	configFile := filepath.Join(configDir, "config.yml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		viper.Set("host", defaultAPIIP)
		viper.Set("port", defaultAPIPort)
		viper.Set("tls", true)
		viper.Set("timeout", defaultTimeout)
		if err := viper.SafeWriteConfig(); err != nil {
			logger.Fatal().AnErr("error", err).Msg("failed to create config file")
		}
	}

	viper.SetDefault("host", defaultAPIIP)
	viper.SetDefault("port", defaultAPIPort)
	viper.SetDefault("tls", true)
	viper.SetDefault("timeout", defaultTimeout)

	viper.SetEnvPrefix("ORION")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		logger.Fatal().AnErr("error", err).Msg("failed to parse config file")
	}
}
