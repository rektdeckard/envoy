package main

import (
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

type Config struct {
	Carriers struct {
		FedEx CarrierConfig `yaml:"fedex"`
		UPS   CarrierConfig `yaml:"ups"`
		USPS  CarrierConfig `yaml:"usps"`
	}
}

type CarrierConfig struct {
	Key    string `yaml:"key"`
	Secret string `yaml:"secret"`
	Extra  string `yaml:"extra"`
}

func initConfig() Config {
	if confPath != "" {
		// Use config file from the flag.
		viper.SetConfigFile(confPath)
	} else {
		// Find dir directory.
		dir, err := ConfigDir()
		if err != nil {
			log.Fatalf("could not locate config dir: %v", err)
		}

		viper.AddConfigPath(dir)
		viper.SetConfigName("envoy")
		viper.SetConfigType("yaml")
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error
		} else {
			log.Fatal("Error reading config file", err)
		}
	}

	viper.AutomaticEnv()

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("unable to decode config: %v", err)
	}

	return config
}
