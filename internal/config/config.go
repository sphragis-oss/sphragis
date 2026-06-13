// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddr      string
	UpstreamBaseURL string
	UpstreamAPIKey  string
	AuditLogPath    string
	CustomTermsFile string
	NERURL          string
	OTSCalendars    []string
}

func FromEnv() Config {
	return Config{
		ListenAddr:      env("SPHRAGIS_LISTEN_ADDR", ":8787"),
		UpstreamBaseURL: env("SPHRAGIS_UPSTREAM_BASE_URL", "https://api.openai.com"),
		UpstreamAPIKey:  os.Getenv("SPHRAGIS_UPSTREAM_API_KEY"),
		AuditLogPath:    env("SPHRAGIS_AUDIT_LOG_PATH", filepath.Join(Home(), "audit.jsonl")),
		CustomTermsFile: os.Getenv("SPHRAGIS_CUSTOM_TERMS_FILE"),
		NERURL:          os.Getenv("SPHRAGIS_NER_URL"),
		OTSCalendars:    splitCSV(os.Getenv("SPHRAGIS_OTS_CALENDARS")),
	}
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

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
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
