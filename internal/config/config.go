// Package config содержит логику чтения конфигурации сервиса гофермарт.
package config

import (
	"flag"
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config содержит параметры конфигурации сервиса гофермарт.
type Config struct {
	RunAddress           string `env:"RUN_ADDRESS"`
	DatabaseURI          string `env:"DATABASE_URI"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
}

// Parse считывает конфигурацию из флагов командной строки и переменных окружения.
func Parse() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	envRunAddress := cfg.RunAddress
	envDatabaseURI := cfg.DatabaseURI
	envAccrualAddress := cfg.AccrualSystemAddress

	flag.StringVar(&cfg.RunAddress, "a", "localhost:8080", "address and port for HTTP server")
	flag.StringVar(&cfg.DatabaseURI, "d", "", "database URI")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", "", "accrual system address")

	flag.Parse()

	if envRunAddress != "" {
		cfg.RunAddress = envRunAddress
	}
	if envDatabaseURI != "" {
		cfg.DatabaseURI = envDatabaseURI
	}
	if envAccrualAddress != "" {
		cfg.AccrualSystemAddress = envAccrualAddress
	}

	if cfg.RunAddress == "" {
		cfg.RunAddress = "localhost:8080"
	}

	return cfg, nil
}
