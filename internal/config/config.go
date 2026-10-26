// Package config loads the project list and runtime settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Project is one repository to track.
type Project struct {
	// Name is the short id used in URLs (/api/projects/{name}).
	Name string `json:"name"`
	// Repo is "owner/repo" on GitHub.
	Repo string `json:"repo"`
	// Kind is "product" (full scoring) or "profile" (freshness and docs only).
	Kind string `json:"kind"`
	// Description is a one-line summary shown on the dashboard.
	Description string `json:"description"`
	// Owner is the person accountable for the project (optional).
	Owner string `json:"owner"`
}

// File is the shape of projects.json.
type File struct {
	LookbackDays int       `json:"lookback_days"`
	Projects     []Project `json:"projects"`
}

// Config is the full runtime configuration.
type Config struct {
	Addr         string
	Token        string
	APIBase      string
	CacheTTL     time.Duration
	LookbackDays int
	Projects     []Project
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Load reads env vars and the projects file.
func Load() (*Config, error) {
	c := &Config{
		Addr:    env("PULSE_ADDR", ":8080"),
		Token:   strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		APIBase: env("GITHUB_API_URL", ""),
	}
	ttl, err := time.ParseDuration(env("PULSE_CACHE_TTL", "10m"))
	if err != nil {
		return nil, fmt.Errorf("PULSE_CACHE_TTL: %w", err)
	}
	c.CacheTTL = ttl
	f, err := ReadFile(env("PULSE_CONFIG", "projects.json"))
	if err != nil {
		return nil, err
	}
	c.LookbackDays = f.LookbackDays
	if v := os.Getenv("PULSE_LOOKBACK_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 7 || n > 365 {
			return nil, errors.New("PULSE_LOOKBACK_DAYS must be a number from 7 to 365")
		}
		c.LookbackDays = n
	}
	c.Projects = f.Projects
	return c, nil
}

// ReadFile parses and validates a projects file.
func ReadFile(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project list: %w", err)
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.LookbackDays == 0 {
		f.LookbackDays = 90
	}
	if len(f.Projects) == 0 {
		return nil, fmt.Errorf("%s lists no projects", path)
	}
	seen := map[string]bool{}
	for i := range f.Projects {
		p := &f.Projects[i]
		if !repoRe.MatchString(p.Repo) {
			return nil, fmt.Errorf("project %d: repo %q must look like owner/repo", i+1, p.Repo)
		}
		if p.Name == "" {
			p.Name = strings.ToLower(strings.SplitN(p.Repo, "/", 2)[1])
		}
		if !nameRe.MatchString(p.Name) {
			return nil, fmt.Errorf("project %q: name must be lowercase letters, digits, - or _", p.Name)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("project name %q is used twice", p.Name)
		}
		seen[p.Name] = true
		switch p.Kind {
		case "":
			p.Kind = "product"
		case "product", "profile":
		default:
			return nil, fmt.Errorf("project %q: kind must be product or profile", p.Name)
		}
	}
	return &f, nil
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
