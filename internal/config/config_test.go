package config

import "testing"

func TestLoadReadsInstanceIdentity(t *testing.T) {
	t.Setenv("BR_REGISTRY_INSTANCE_ID", "frontier-community-registry")
	t.Setenv("BR_REGISTRY_API_VERSION", "v1-community")

	cfg := Load()
	if cfg.InstanceID != "frontier-community-registry" {
		t.Fatalf("expected configured instance id, got %q", cfg.InstanceID)
	}
	if cfg.APIVersion != "v1-community" {
		t.Fatalf("expected configured API version, got %q", cfg.APIVersion)
	}
}

func TestLoadReadsCloudflareAccessAdminAuth(t *testing.T) {
	t.Setenv("BR_REGISTRY_ACCESS_TEAM_DOMAIN", "https://team.cloudflareaccess.com")
	t.Setenv("BR_REGISTRY_ACCESS_AUD", "aud-tag")
	t.Setenv("BR_REGISTRY_ACCESS_CERTS_URL", "https://team.cloudflareaccess.com/cdn-cgi/access/certs")

	cfg := Load()
	if cfg.AccessTeamDomain != "https://team.cloudflareaccess.com" {
		t.Fatalf("expected configured Access team domain, got %q", cfg.AccessTeamDomain)
	}
	if cfg.AccessAudience != "aud-tag" {
		t.Fatalf("expected configured Access audience, got %q", cfg.AccessAudience)
	}
	if cfg.AccessCertsURL != "https://team.cloudflareaccess.com/cdn-cgi/access/certs" {
		t.Fatalf("expected configured Access certs URL, got %q", cfg.AccessCertsURL)
	}
}

func TestValidateRejectsPartialCloudflareAccessAdminAuth(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "missing audience",
			cfg: Config{
				Address:          "127.0.0.1:8080",
				DatabaseURL:      "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable",
				ArtefactRoot:     "artefacts",
				InstanceID:       "black-relay-registry",
				APIVersion:       "v1",
				AccessTeamDomain: "https://team.cloudflareaccess.com",
			},
		},
		{
			name: "missing team domain",
			cfg: Config{
				Address:        "127.0.0.1:8080",
				DatabaseURL:    "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable",
				ArtefactRoot:   "artefacts",
				InstanceID:     "black-relay-registry",
				APIVersion:     "v1",
				AccessAudience: "aud-tag",
			},
		},
		{
			name: "certs URL without validator settings",
			cfg: Config{
				Address:        "127.0.0.1:8080",
				DatabaseURL:    "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable",
				ArtefactRoot:   "artefacts",
				InstanceID:     "black-relay-registry",
				APIVersion:     "v1",
				AccessCertsURL: "https://team.cloudflareaccess.com/cdn-cgi/access/certs",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("expected partial Access configuration to be rejected")
			}
		})
	}
}

func TestValidateRejectsBlankInstanceID(t *testing.T) {
	cfg := Load()
	cfg.InstanceID = " "

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected blank instance id to be rejected")
	}
}
