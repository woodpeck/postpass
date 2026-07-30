package postpass

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type DatabaseConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	DatabaseName string `yaml:"database_name"`
}

type MetricsConfig struct {
	Enabled                     bool      `yaml:"enabled"`
	EstCostBuckets              []float64 `yaml:"est_cost_buckets"`
	QueryDurationBuckets        []float64 `yaml:"query_duration_buckets"`
}

type PostpassConfig struct {
	Database             DatabaseConfig `yaml:"database"`
	ListenPort           int            `yaml:"listen_port"`
	QuickMediumThreshold int            `yaml:"quick_medium_threshold"`
	MediumSlowThreshold  int            `yaml:"medium_slow_threshold"`
	Metrics              MetricsConfig  `yaml:"metrics"`
}

func DefaultConfig() PostpassConfig {
	return PostpassConfig{
		Database: DatabaseConfig{
			Host:         "localhost",
			Port:         5432,
			User:         "readonly",
			Password:     "readonly",
			DatabaseName: "gis",
		},
		ListenPort:           8081,
		QuickMediumThreshold: 150,
		MediumSlowThreshold:  150000,
		Metrics: MetricsConfig{
			Enabled:                     true,
			EstCostBuckets:              []float64{1.0, 10.0, 100.0},
			QueryDurationBuckets:        []float64{0.1, 1.0, 10.0, 100.0},
		},
	}
}

func LoadConfig(path *string) (PostpassConfig, error) {
	cfg := DefaultConfig()

	if path == nil {
		return cfg, nil
	}

	data, err := os.ReadFile(*path)
	if err != nil {
		return cfg, fmt.Errorf("error reading config file %s: %w", *path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("error parsing config file %s: %w", *path, err)
	}

	return cfg, nil
}
