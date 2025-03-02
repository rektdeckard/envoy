package main

import (
	"log"
	"os"
	"path"
	"runtime"

	"github.com/spf13/viper"
)

func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		dir = path.Join(dir, "com.rektsoft.envoy")
	default:
		dir = path.Join(dir, "envoy")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	return dir, nil
}

func InitConfig() error {
	if cfg != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfg)
	} else {
		// Find dir directory.
		dir, err := ConfigDir()
		if err != nil {
			return err
		}

		viper.AddConfigPath(dir)
		viper.SetConfigName("envoy")
		viper.SetConfigType("toml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error
		} else {
			log.Fatal("Error reading config file", err)
		}
	}

	return nil
}
