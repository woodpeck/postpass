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
	TaskQueueDurationBuckets    []float64 `yaml:"task_queue_duration_buckets"`
	TotalDurationBuckets        []float64 `yaml:"total_duration_buckets"`
	EstCostDurationRatioBuckets []float64 `yaml:"est_cost_query_duration_ratio_buckets"`
	RespSizeBuckets             []float64 `yaml:"resp_size_buckets"`
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
			TaskQueueDurationBuckets:    []float64{0.1, 1.0, 10.0, 100.0},
			TotalDurationBuckets:        []float64{1e-4, 1e-3, 1e-2, 1e-1, 1.0, 10.0, 100.0},
			EstCostDurationRatioBuckets: []float64{1. / 10, 1. / 2, 1., 2. / 1, 10. / 1, 1e+2, 1e+3, 1e+4, 1e+5, 1e+6},
			RespSizeBuckets:             []float64{100, 500, 1000, 10e3, 100e3, 1e6},
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
