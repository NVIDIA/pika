package config

import (
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	DefaultConfigPath = "/etc/config/notifyMaintenanceConfig.yaml"
)

type Config struct {
	SLAPeriod time.Duration `yaml:"slaPeriod,omitempty"`
}

func (c *Config) Validate() error {
	if c.SLAPeriod <= 0 {
		return errors.New("slaPeriod is not defined")
	}
	return nil
}

func ReadConfig(configPath string) (Config, error) {
	config := Config{}

	content, err := os.ReadFile(configPath)
	if err != nil {
		logf.Log.Error(err, "config file not found")
		return config, err
	}

	if err = yaml.Unmarshal(content, &config); err != nil {
		logf.Log.Error(err, "invalid config file")
		return config, err
	}

	if err := config.Validate(); err != nil {
		logf.Log.Error(err, "failed to validate config")
		return config, err
	}

	return config, nil
}
