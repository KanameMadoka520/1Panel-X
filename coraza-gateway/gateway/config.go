package gateway

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// SiteConfig maps one protected host to its origin upstream. The agent generates
// this from the managed-website registry (Phase 21); the gateway loads it to
// route each request to the right origin by Host.
type SiteConfig struct {
	WebsiteID uint   `json:"websiteId,omitempty"`
	Alias     string `json:"alias,omitempty"`
	Host      string `json:"host"`
	Upstream  string `json:"upstream"`
	Mode      Mode   `json:"mode,omitempty"`
}

// Config is the gateway's versioned per-site routing table.
type Config struct {
	Version    int          `json:"version,omitempty"`
	Generation string       `json:"generation,omitempty"`
	Sites      []SiteConfig `json:"sites"`
}

// LoadConfig reads and validates the routing config from a file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}

// ParseConfig validates the routing config: every site needs a non-empty,
// canonical host and an HTTP(S) origin URL. Hosts are normalized exactly as
// Router does, and duplicate normalized hosts are rejected instead of allowing
// the last entry to silently select a different origin (W12).
func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("waf config: invalid json: %w", err)
	}
	seen := make(map[string]struct{}, len(c.Sites))
	if c.Version != 0 && c.Version != 1 {
		return Config{}, fmt.Errorf("waf config: unsupported version %d", c.Version)
	}
	for i := range c.Sites {
		host := normalizeHost(c.Sites[i].Host)
		if host == "" {
			return Config{}, fmt.Errorf("waf config: site %d has empty or invalid host", i)
		}
		if _, ok := seen[host]; ok {
			return Config{}, fmt.Errorf("waf config: duplicate normalized host %q", host)
		}
		seen[host] = struct{}{}
		c.Sites[i].Host = host
		if c.Sites[i].Mode != "" && c.Sites[i].Mode != ModeBlock && c.Sites[i].Mode != ModeDetection {
			return Config{}, fmt.Errorf("waf config: site %q has invalid mode %q", host, c.Sites[i].Mode)
		}

		u, err := url.Parse(c.Sites[i].Upstream)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" ||
			(u.Scheme != "http" && u.Scheme != "https") {
			return Config{}, fmt.Errorf("waf config: site %q has invalid HTTP(S) upstream %q", host, c.Sites[i].Upstream)
		}
		if strings.TrimSpace(u.Hostname()) == "" {
			return Config{}, fmt.Errorf("waf config: site %q has invalid HTTP(S) upstream %q", host, c.Sites[i].Upstream)
		}
	}
	return c, nil
}
