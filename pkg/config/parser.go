package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// LoadConfig loads and parses a TOML configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML config: %w", err)
	}

	return &cfg, nil
}

// LoadConfigFromString parses TOML configuration from a string
func LoadConfigFromString(data string) (*Config, error) {
	var cfg Config
	if err := toml.Unmarshal([]byte(data), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML config: %w", err)
	}

	return &cfg, nil
}
