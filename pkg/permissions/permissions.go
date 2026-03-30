// Package permissions implements the AxiOS tiered trust model.
package permissions

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Tier string

const (
	Trusted          Tier = "trusted"
	ApprovalRequired Tier = "approval_required"
	Prohibited       Tier = "prohibited"
)

// Config holds the permission tiers loaded from permissions.yaml.
type Config struct {
	Tiers struct {
		Trusted          []string `yaml:"trusted"`
		ApprovalRequired []string `yaml:"approval_required"`
		Prohibited       []string `yaml:"prohibited"`
	} `yaml:"tiers"`
}

// LoadConfig reads and parses a permissions.yaml file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read permissions config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse permissions config: %w", err)
	}
	return &cfg, nil
}

// Check returns the trust tier for a given operation.
func (c *Config) Check(operation string) Tier {
	for _, op := range c.Tiers.Prohibited {
		if op == operation {
			return Prohibited
		}
	}
	for _, op := range c.Tiers.ApprovalRequired {
		if op == operation {
			return ApprovalRequired
		}
	}
	for _, op := range c.Tiers.Trusted {
		if op == operation {
			return Trusted
		}
	}
	// Unknown operations require approval by default
	return ApprovalRequired
}
