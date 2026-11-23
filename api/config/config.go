// Package config loads the WattLedger API connection settings (application.yaml).
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile is used when the CONFIG_PATH environment variable is empty.
const DefaultConfigFile = "application.yaml"

// OrgConfig holds the identity and peer of one consortium organization.
type OrgConfig struct {
	Name             string `yaml:"name" json:"name"`
	DisplayName      string `yaml:"displayName" json:"displayName"`
	MSPID            string `yaml:"mspID" json:"mspID"`
	PeerEndpoint     string `yaml:"peerEndpoint" json:"peerEndpoint"`
	PeerHostOverride string `yaml:"peerHostOverride" json:"-"`
	TLSCACert        string `yaml:"tlsCACert" json:"-"`
	CertPath         string `yaml:"certPath" json:"-"`
	KeyPath          string `yaml:"keyPath" json:"-"`
}

// Config is the full API configuration.
type Config struct {
	Channel    string `yaml:"channel"`
	DefaultOrg string `yaml:"defaultOrg"`
	Chaincodes struct {
		Governance string `yaml:"governance"`
		Energy     string `yaml:"energy"`
	} `yaml:"chaincodes"`
	Orgs []OrgConfig `yaml:"orgs"`
}

// Load reads and validates the configuration file.
func Load() (*Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = DefaultConfigFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config '%s': %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if cfg.Channel == "" || len(cfg.Orgs) == 0 {
		return nil, fmt.Errorf("config must define a channel and at least one org")
	}
	if cfg.DefaultOrg == "" {
		cfg.DefaultOrg = cfg.Orgs[0].Name
	}
	return &cfg, nil
}
