package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Upload     UploadConfig     `yaml:"upload"`
	Export     ExportConfig     `yaml:"export"`
	Scheduler  SchedulerConfig  `yaml:"scheduler"`
	Commission CommissionConfig `yaml:"commission"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type UploadConfig struct {
	Dir       string `yaml:"dir"`
	MaxSizeMB int    `yaml:"max_size_mb"`
}

type ExportConfig struct {
	Dir string `yaml:"dir"`
}

type SchedulerConfig struct {
	CutoffHour   int `yaml:"cutoff_hour"`
	CommissionDay int `yaml:"commission_day"`
}

type CommissionConfig struct {
	Rate float64 `yaml:"rate"`
}

var C Config

func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &C)
}
