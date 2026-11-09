// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific lang

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

func initConfig() error {
	home, homedirErr := os.UserHomeDir()
	if homedirErr != nil {
		return fmt.Errorf("failed to detect home dir: %w", homedirErr)
	}
	configDir := filepath.Join(home, ".orion")
	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yml")

	if err := os.MkdirAll(configDir, readWrite); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	// Defaults, not viper.Set: a value set with viper.Set overrules the flags
	viper.SetDefault(flagHost, defaultAPIIP)
	viper.SetDefault(flagPort, defaultAPIPort)
	viper.SetDefault(flagPKIDir, defaultCLIPKIDir)
	viper.SetDefault(flagTimeout, defaultTimeout)

	configFile := filepath.Join(configDir, "config.yml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
	}

	viper.SetEnvPrefix("ORION")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	return nil
}
