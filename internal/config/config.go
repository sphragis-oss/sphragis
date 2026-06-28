// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddr       string
	AnthropicBaseURL string
	OpenAIBaseURL    string
	GoogleBaseURL    string
	UpstreamBaseURL  string
	UpstreamAPIKey   string
	AuditLogPath     string
	CustomTermsFile  string
	NERURL           string
	OTSCalendars     []string
	VaultKeyfile     string
	VaultPath        string
	EUPack           bool
	BuiltinNER       bool
	RouteAutodetect  bool
}

// defaults returns the baseline config before file and env layering.
func defaults() Config {
	return Config{
		ListenAddr:       ":8787",
		AnthropicBaseURL: "https://api.anthropic.com",
		OpenAIBaseURL:    "https://api.openai.com",
		GoogleBaseURL:    "https://generativelanguage.googleapis.com",
		AuditLogPath:     filepath.Join(Home(), "audit.jsonl"),
		VaultPath:        filepath.Join(Home(), "vault.bin"),
		RouteAutodetect:  true,
	}
}

// Load resolves config from defaults, then an optional YAML file, then env vars.
// Precedence is env > file > default, so existing env-only setups are unchanged.
func Load() (Config, error) {
	c := defaults()
	if p := configPath(); p != "" {
		if err := applyFile(&c, p); err != nil {
			return c, fmt.Errorf("config %s: %w", p, err)
		}
	}
	applyEnv(&c)
	return c, nil
}

// FromEnv keeps the env-only path (defaults + env), used where no file is wanted.
func FromEnv() Config {
	c := defaults()
	applyEnv(&c)
	return c
}

// configPath returns SPHRAGIS_CONFIG, else $SPHRAGIS_HOME/sphragis.yaml if it exists.
func configPath() string {
	if p := os.Getenv("SPHRAGIS_CONFIG"); p != "" {
		return p
	}
	def := filepath.Join(Home(), "sphragis.yaml")
	if _, err := os.Stat(def); err == nil {
		return def
	}
	return ""
}

func applyEnv(c *Config) {
	setEnv(&c.ListenAddr, "SPHRAGIS_LISTEN_ADDR")
	setEnv(&c.AnthropicBaseURL, "SPHRAGIS_ANTHROPIC_BASE_URL")
	setEnv(&c.OpenAIBaseURL, "SPHRAGIS_OPENAI_BASE_URL")
	setEnv(&c.GoogleBaseURL, "SPHRAGIS_GOOGLE_BASE_URL")
	setEnv(&c.UpstreamBaseURL, "SPHRAGIS_UPSTREAM_BASE_URL")
	setEnv(&c.UpstreamAPIKey, "SPHRAGIS_UPSTREAM_API_KEY")
	setEnv(&c.AuditLogPath, "SPHRAGIS_AUDIT_LOG_PATH")
	setEnv(&c.CustomTermsFile, "SPHRAGIS_CUSTOM_TERMS_FILE")
	setEnv(&c.NERURL, "SPHRAGIS_NER_URL")
	setEnv(&c.VaultKeyfile, "SPHRAGIS_VAULT_KEYFILE")
	setEnv(&c.VaultPath, "SPHRAGIS_VAULT_PATH")
	if v := os.Getenv("SPHRAGIS_OTS_CALENDARS"); v != "" {
		c.OTSCalendars = splitCSV(v)
	}
	if v := os.Getenv("SPHRAGIS_EU_PACK"); v != "" {
		c.EUPack = truthy(v)
	}
	if v := os.Getenv("SPHRAGIS_NER_BUILTIN"); v != "" {
		c.BuiltinNER = truthy(v)
	}
	if v := os.Getenv("SPHRAGIS_ROUTE_AUTODETECT"); v != "" {
		c.RouteAutodetect = truthy(v)
	}
}

// truthy parses a boolean-ish string ("true", "1", "yes", "on").
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

func setEnv(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// applyFile overlays a flat-YAML config file onto c, ignoring a missing file.
func applyFile(c *Config, path string) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	scalars, lists, err := parseFlatYAML(b)
	if err != nil {
		return err
	}
	str := map[string]*string{
		"listen_addr":        &c.ListenAddr,
		"anthropic_base_url": &c.AnthropicBaseURL,
		"openai_base_url":    &c.OpenAIBaseURL,
		"google_base_url":    &c.GoogleBaseURL,
		"upstream_base_url":  &c.UpstreamBaseURL,
		"upstream_api_key":   &c.UpstreamAPIKey,
		"audit_log_path":     &c.AuditLogPath,
		"custom_terms_file":  &c.CustomTermsFile,
		"ner_url":            &c.NERURL,
		"vault_keyfile":      &c.VaultKeyfile,
		"vault_path":         &c.VaultPath,
	}
	for k, v := range scalars {
		if dst, ok := str[k]; ok {
			*dst = v
		} else if k == "eu_pack" {
			c.EUPack = truthy(v)
		} else if k == "ner_builtin" {
			c.BuiltinNER = truthy(v)
		} else if k == "route_autodetect" {
			c.RouteAutodetect = truthy(v)
		} else if k != "ots_calendars" {
			return fmt.Errorf("unknown key %q", k)
		}
	}
	if v, ok := lists["ots_calendars"]; ok {
		c.OTSCalendars = v
	}
	return nil
}

// parseFlatYAML parses a flat "key: value" subset with one level of "- item" lists.
func parseFlatYAML(b []byte) (map[string]string, map[string][]string, error) {
	scalars := map[string]string{}
	lists := map[string][]string{}
	var listKey string
	for n, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
			if listKey == "" {
				return nil, nil, fmt.Errorf("line %d: list item without a key", n+1)
			}
			lists[listKey] = append(lists[listKey], unquote(item))
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil, nil, fmt.Errorf("line %d: not key: value", n+1)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if val == "" {
			listKey = key
			continue
		}
		listKey = ""
		scalars[key] = unquote(val)
	}
	return scalars, lists, nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// splitCSV splits a comma-separated env value, trimming blanks.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LoadCustomTerms reads one term per line, skipping blanks and # comments.
func LoadCustomTerms(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out, nil
}

// Home returns the sphragis state directory (SPHRAGIS_HOME or ~/.sphragis).
func Home() string {
	if h := os.Getenv("SPHRAGIS_HOME"); h != "" {
		return h
	}
	hd, err := os.UserHomeDir()
	if err != nil {
		return ".sphragis"
	}
	return filepath.Join(hd, ".sphragis")
}
