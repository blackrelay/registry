package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address          string
	DatabaseURL      string
	ArtefactRoot     string
	AdminToken       string
	AccessTeamDomain string
	AccessAudience   string
	AccessCertsURL   string
	InstanceID       string
	APIVersion       string
	ReadinessTimeout time.Duration
}

func Load() Config {
	return Config{
		Address:          envString("BR_REGISTRY_ADDR", "127.0.0.1:8080"),
		DatabaseURL:      envString("DATABASE_URL", "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"),
		ArtefactRoot:     envString("BR_REGISTRY_ARTEFACT_ROOT", "artefacts"),
		AdminToken:       os.Getenv("BR_REGISTRY_ADMIN_TOKEN"),
		AccessTeamDomain: strings.TrimSpace(os.Getenv("BR_REGISTRY_ACCESS_TEAM_DOMAIN")),
		AccessAudience:   strings.TrimSpace(os.Getenv("BR_REGISTRY_ACCESS_AUD")),
		AccessCertsURL:   strings.TrimSpace(os.Getenv("BR_REGISTRY_ACCESS_CERTS_URL")),
		InstanceID:       envString("BR_REGISTRY_INSTANCE_ID", "black-relay-registry"),
		APIVersion:       envString("BR_REGISTRY_API_VERSION", "v1"),
		ReadinessTimeout: envDuration("BR_REGISTRY_READY_TIMEOUT", 2*time.Second),
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Address) == "" {
		return errors.New("listen address is required")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if strings.TrimSpace(c.ArtefactRoot) == "" {
		return errors.New("artefact root is required")
	}
	if strings.TrimSpace(c.InstanceID) == "" {
		return errors.New("registry instance id is required")
	}
	if strings.TrimSpace(c.APIVersion) == "" {
		return errors.New("API version is required")
	}
	if err := c.validateAccessAdminAuth(); err != nil {
		return err
	}
	return nil
}

func (c Config) validateAccessAdminAuth() error {
	teamDomain := strings.TrimSpace(c.AccessTeamDomain)
	audience := strings.TrimSpace(c.AccessAudience)
	certsURL := strings.TrimSpace(c.AccessCertsURL)
	if teamDomain == "" && audience == "" && certsURL == "" {
		return nil
	}
	if teamDomain == "" {
		return errors.New("cloudflare access team domain is required when Access admin auth is configured")
	}
	if audience == "" {
		return errors.New("cloudflare access AUD is required when Access admin auth is configured")
	}
	return nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
