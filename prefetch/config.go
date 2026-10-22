package prefetch

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type Config struct {
	Enabled           bool   `json:"enabled"`
	SiloURL           string `json:"-"` // Runtime-only address discovered from the host.
	SiloAPIKey        string `json:"silo_api_key"`
	SonarrURL         string `json:"sonarr_url"`
	SonarrAPIKey      string `json:"sonarr_api_key"`
	Sonarr4KURL       string `json:"sonarr_4k_url"`
	Sonarr4KAPIKey    string `json:"sonarr_4k_api_key"`
	EpisodesAhead     int    `json:"episodes_ahead"`
	MinWatchedPercent int    `json:"min_watched_percent"`
	IncludeUsers      string `json:"include_users"`
	ExcludeUsers      string `json:"exclude_users"`
	IncludeTags       string `json:"include_tags"`
	ExcludeTags       string `json:"exclude_tags"`
}

func defaultConfig() Config {
	return Config{EpisodesAhead: 2, MinWatchedPercent: 10}
}

func DecodeConfig(data []byte) (Config, error) {
	c := defaultConfig()
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	// Accept and discard this older saved field when upgrading an installation.
	var stored struct {
		*Config
		LegacySiloURL json.RawMessage `json:"silo_url"`
	}
	stored.Config = &c
	err := d.Decode(&stored)
	return c, err
}

func (c Config) Validate() error {
	if c.EpisodesAhead < 1 || c.EpisodesAhead > 100 {
		return fmt.Errorf("episodes_ahead must be between 1 and 100")
	}
	if c.MinWatchedPercent < 0 || c.MinWatchedPercent > 100 {
		return fmt.Errorf("min_watched_percent must be between 0 and 100")
	}
	if c.SiloAPIKey == "" {
		return fmt.Errorf("Silo API key is required")
	}
	for _, endpoint := range []struct {
		name, address, key string
		required           bool
	}{
		{"Sonarr", c.SonarrURL, c.SonarrAPIKey, true},
		{"Sonarr 4K", c.Sonarr4KURL, c.Sonarr4KAPIKey, false},
	} {
		if !endpoint.required && endpoint.address == "" && endpoint.key == "" {
			continue
		}
		if endpoint.key == "" {
			return fmt.Errorf("%s API key is required", endpoint.name)
		}
		if err := validURL(endpoint.address); err != nil {
			return fmt.Errorf("%s URL must be an absolute http(s) base URL without credentials, query, or fragment", endpoint.name)
		}
	}
	return nil
}

func validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid base URL")
	}
	return nil
}

func list(raw string) []string {
	var out []string
	for _, v := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func matches(entries []string, values ...string) bool {
	for _, e := range entries {
		for _, v := range values {
			if strings.EqualFold(e, v) && v != "" {
				return true
			}
		}
	}
	return false
}

func (c Config) permitsUser(s Session) bool {
	ids := []string{s.UserID, s.Username, s.ProfileID, s.ProfileName}
	return !matches(list(c.ExcludeUsers), ids...) && (len(list(c.IncludeUsers)) == 0 || matches(list(c.IncludeUsers), ids...))
}
