package gateway

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

// SiteConfig maps one protected host to its origin upstream. The agent generates
// this from the managed-website registry (Phase 21); the gateway loads it to
// route each request to the right origin by Host.
type SiteConfig struct {
	Host     string `json:"host"`
	Upstream string `json:"upstream"`
}

// Config is the gateway's per-site routing table.
type Config struct {
	Sites []SiteConfig `json:"sites"`
}

// LoadConfig reads and validates the routing config from a file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}

// ParseConfig validates the routing config: every site needs a non-empty host
// and a parseable absolute upstream URL. Pure — no I/O — so it is unit-testable.
func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("waf config: invalid json: %w", err)
	}
	for i := range c.Sites {
		if c.Sites[i].Host == "" {
			return Config{}, fmt.Errorf("waf config: site %d has empty host", i)
		}
		u, err := url.Parse(c.Sites[i].Upstream)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return Config{}, fmt.Errorf("waf config: site %q has invalid upstream %q", c.Sites[i].Host, c.Sites[i].Upstream)
		}
	}
	return c, nil
}
