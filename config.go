package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tidwall/jsonc"
)

type Config struct {
	Listen                 string     `json:"listen"`
	DefaultCooldownSeconds int        `json:"default_cooldown_seconds"`
	Providers              []Provider `json:"providers"`
}

type Provider struct {
	Name            string            `json:"name"`
	BaseURL         string            `json:"base_url"`
	AuthStyle       string            `json:"auth_style"`
	Tier            int               `json:"tier"`
	OverrideModel   string            `json:"override_model,omitempty"`
	CooldownSeconds int               `json:"cooldown_seconds,omitempty"`
	ExtraHeaders    map[string]string `json:"extra_headers,omitempty"`
	Keys            []string          `json:"keys"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	jsonBytes := jsonc.ToJSON(data)
	var cfg Config
	if err := json.Unmarshal(jsonBytes, &cfg); err != nil {
		return nil, fmt.Errorf("config parse error: %w", err)
	}
	return &cfg, nil
}
