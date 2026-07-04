// Package config loads and saves the bootstrap configuration (YAML) written by
// the installer and read by every command. See SPEC §7.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"archivesync/internal/crypto"
	"gopkg.in/yaml.v3"
)

// Config is the bootstrap configuration.
type Config struct {
	Listen          string    `yaml:"listen"`
	DataDir         string    `yaml:"data_dir"`
	BaseURL         string    `yaml:"base_url"`
	MasterKey       string    `yaml:"master_key"`
	SessionTTLHours int       `yaml:"session_ttl_hours"`
	IAM             IAMConfig `yaml:"iam"`
}

// IAMConfig holds TransCircle IAM / OIDC settings.
type IAMConfig struct {
	Issuer             string   `yaml:"issuer"`
	ClientID           string   `yaml:"client_id"`
	ClientSecret       string   `yaml:"client_secret"`
	RedirectURL        string   `yaml:"redirect_url"`
	AppKey             string   `yaml:"app_key"`
	Scopes             []string `yaml:"scopes"`
	RequiredPermission string   `yaml:"required_permission"`
	RequiredRole       string   `yaml:"required_role"`
}

// Default returns a config populated with sensible defaults.
func Default() *Config {
	return &Config{
		Listen:          ":8787",
		DataDir:         "./data",
		BaseURL:         "http://localhost:8787",
		SessionTTLHours: 24, // admin re-authenticates via IAM SSO daily (bounds revocation lag)
		IAM: IAMConfig{
			Issuer:      "https://iam.transcircle.org",
			RedirectURL: "http://localhost:8787/api/auth/callback",
			AppKey:      "archive-sync",
			Scopes:      []string{"openid", "profile", "email", "tc.permissions"},
		},
	}
}

// ApplyDefaults fills empty fields with defaults.
func (c *Config) ApplyDefaults() {
	d := Default()
	if c.Listen == "" {
		c.Listen = d.Listen
	}
	// Coerce a bare port (e.g. "18787") into a valid ":port" listen address,
	// otherwise net.Listen fails with "missing port in address".
	if !strings.Contains(c.Listen, ":") {
		c.Listen = ":" + c.Listen
	}
	if c.DataDir == "" {
		c.DataDir = d.DataDir
	}
	if c.BaseURL == "" {
		c.BaseURL = d.BaseURL
	}
	if c.SessionTTLHours == 0 {
		c.SessionTTLHours = d.SessionTTLHours
	}
	if c.IAM.Issuer == "" {
		c.IAM.Issuer = d.IAM.Issuer
	}
	if c.IAM.AppKey == "" {
		c.IAM.AppKey = d.IAM.AppKey
	}
	if c.IAM.RedirectURL == "" {
		c.IAM.RedirectURL = c.BaseURL + "/api/auth/callback"
	}
	if len(c.IAM.Scopes) == 0 {
		c.IAM.Scopes = d.IAM.Scopes
	}
}

// EnsureMasterKey generates a master key if none is set. Returns true if a new
// key was generated (caller should persist the config).
func (c *Config) EnsureMasterKey() (bool, error) {
	if c.MasterKey != "" {
		return false, nil
	}
	k, err := crypto.GenerateKey()
	if err != nil {
		return false, err
	}
	c.MasterKey = k
	return true, nil
}

// Cipher builds the field cipher from the master key (nil if unset).
func (c *Config) Cipher() (*crypto.Cipher, error) {
	return crypto.NewCipherFromBase64(c.MasterKey)
}

// Load reads and parses the config file at path, applying defaults.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	c.ApplyDefaults()
	return &c, nil
}

// Save writes the config to path (creating parent dirs), with 0600 perms.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// DefaultPath returns the config path from $ARCHIVE_SYNC_CONFIG or ./config.yaml.
func DefaultPath() string {
	if p := os.Getenv("ARCHIVE_SYNC_CONFIG"); p != "" {
		return p
	}
	return "config.yaml"
}

// DBPath returns the sqlite database path inside DataDir.
func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "archivesync.db") }

// TmpDir returns a working directory for building archives.
func (c *Config) TmpDir() string { return filepath.Join(c.DataDir, "tmp") }
