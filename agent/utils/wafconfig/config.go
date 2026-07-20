package wafconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

const Version = 1

type Mode string

const (
	ModeBlock     Mode = "block"
	ModeDetection Mode = "detection"
)

// Site is the agent-side input for one explicitly enabled reverse-proxy site.
// The caller remains responsible for loading domains from the managed registry.
type Site struct {
	Website model.Website
	Domains []model.WebsiteDomain
	Enabled bool
	Mode    Mode
}

// GatewayConfig mirrors the data-plane JSON contract without importing the
// separate coraza-gateway Go module.
type GatewayConfig struct {
	Version    int           `json:"version"`
	Generation string        `json:"generation,omitempty"`
	Sites      []GatewaySite `json:"sites"`
}

type GatewaySite struct {
	WebsiteID uint   `json:"websiteId"`
	Alias     string `json:"alias"`
	Host      string `json:"host"`
	Upstream  string `json:"upstream"`
	Mode      Mode   `json:"mode"`
}

// Build creates a deterministic routing table from the panel's authoritative
// website registry. v1 supports reverse-proxy sites only and rejects any host
// collision after port-insensitive normalization (W12).
func Build(sites []Site) (GatewayConfig, error) {
	cfg := GatewayConfig{Version: Version, Sites: make([]GatewaySite, 0)}
	seen := make(map[string]string)
	for _, input := range sites {
		if !input.Enabled {
			continue
		}
		website := input.Website
		if website.Type != constant.Proxy {
			return GatewayConfig{}, fmt.Errorf("waf config: website %q has unsupported type %q", website.Alias, website.Type)
		}
		alias := strings.TrimSpace(website.Alias)
		if alias == "" {
			return GatewayConfig{}, fmt.Errorf("waf config: website %d has empty alias", website.ID)
		}
		upstream, err := normalizeUpstream(website.Proxy)
		if err != nil {
			return GatewayConfig{}, fmt.Errorf("waf config: website %q: %w", alias, err)
		}
		mode := input.Mode
		if mode == "" {
			mode = ModeDetection
		}
		if mode != ModeBlock && mode != ModeDetection {
			return GatewayConfig{}, fmt.Errorf("waf config: website %q has invalid mode %q", alias, mode)
		}
		if len(input.Domains) == 0 {
			return GatewayConfig{}, fmt.Errorf("waf config: website %q has no managed domains", alias)
		}

		for _, domain := range input.Domains {
			host, err := normalizeHost(domain.Domain)
			if err != nil {
				return GatewayConfig{}, fmt.Errorf("waf config: website %q: %w", alias, err)
			}
			if prior, ok := seen[host]; ok {
				return GatewayConfig{}, fmt.Errorf("waf config: host %q is shared by websites %q and %q", host, prior, alias)
			}
			seen[host] = alias
			cfg.Sites = append(cfg.Sites, GatewaySite{
				WebsiteID: website.ID,
				Alias:     alias,
				Host:      host,
				Upstream:  upstream,
				Mode:      mode,
			})
		}
	}

	sort.Slice(cfg.Sites, func(i, j int) bool {
		if cfg.Sites[i].Host != cfg.Sites[j].Host {
			return cfg.Sites[i].Host < cfg.Sites[j].Host
		}
		return cfg.Sites[i].WebsiteID < cfg.Sites[j].WebsiteID
	})
	generationInput, err := json.Marshal(cfg.Sites)
	if err != nil {
		return GatewayConfig{}, err
	}
	digest := sha256.Sum256(generationInput)
	cfg.Generation = hex.EncodeToString(digest[:])
	return cfg, nil
}

func Marshal(cfg GatewayConfig) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}

func normalizeUpstream(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty origin upstream")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid HTTP(S) origin upstream %q", raw)
	}
	if strings.TrimSpace(u.Hostname()) == "" {
		return "", fmt.Errorf("invalid HTTP(S) origin upstream %q", raw)
	}
	return u.String(), nil
}

// normalizeHost deliberately removes the port: the current gateway routes by
// hostname. This makes same-host/different-port websites an explicit conflict
// instead of a silent last-entry-wins bypass.
func normalizeHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" {
		return "", fmt.Errorf("empty domain")
	}
	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}
	if strings.HasPrefix(host, "[") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		} else if strings.HasSuffix(host, "]") {
			host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		} else {
			return "", fmt.Errorf("invalid domain %q", raw)
		}
	} else if strings.Count(host, ":") > 1 {
		return "", fmt.Errorf("IPv6 domain %q must be bracketed", raw)
	} else if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if i := strings.LastIndexByte(host, ':'); i >= 0 {
		port := host[i+1:]
		if port == "" {
			return "", fmt.Errorf("invalid domain %q", raw)
		}
		for _, c := range port {
			if c < '0' || c > '9' {
				return "", fmt.Errorf("invalid domain %q", raw)
			}
		}
		host = host[:i]
	}
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" || strings.ContainsAny(host, "/\\?#@") {
		return "", fmt.Errorf("invalid domain %q", raw)
	}
	return host, nil
}
