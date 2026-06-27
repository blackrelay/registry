package main

import (
	"testing"

	"github.com/blackrelay/registry/internal/auth"
	"github.com/blackrelay/registry/internal/config"
)

func TestAdminAuthorizerFromConfigUsesCloudflareAccessWhenConfigured(t *testing.T) {
	cfg := config.Config{
		AccessTeamDomain: "https://team.cloudflareaccess.com",
		AccessAudience:   "aud-tag",
		AccessCertsURL:   "https://team.cloudflareaccess.com/cdn-cgi/access/certs",
	}

	authorizer := adminAuthorizerFromConfig(cfg)
	validator, ok := authorizer.(*auth.CloudflareAccessValidator)
	if !ok {
		t.Fatalf("expected Cloudflare Access validator, got %T", authorizer)
	}
	if validator.TeamDomain != cfg.AccessTeamDomain || validator.Audience != cfg.AccessAudience || validator.CertsURL != cfg.AccessCertsURL {
		t.Fatalf("unexpected validator configuration: %#v", validator)
	}
}

func TestAdminAuthorizerFromConfigIsNilWithoutCloudflareAccess(t *testing.T) {
	if authorizer := adminAuthorizerFromConfig(config.Config{}); authorizer != nil {
		t.Fatalf("expected no production authorizer by default, got %T", authorizer)
	}
}
