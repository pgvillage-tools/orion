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

	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.Fatal().AnErr("error", err).Msg("failed to create config dir")
	}
	configFile := filepath.Join(configDir, "config.yml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		viper.Set("host", "127.0.0.1")
		viper.Set("port", 8443)
		viper.Set("tls", true)
		if err := viper.SafeWriteConfig(); err != nil {
			logger.Fatal().AnErr("error", err).Msg("failed to create config file")
		}
	}

	viper.SetDefault("host", "127.0.0.1")
	viper.SetDefault("port", 8443)
	viper.SetDefault("tls", true)

	viper.SetEnvPrefix("ORION")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		logger.Fatal().AnErr("error", err).Msg("failed to parse config file")
	}
}
