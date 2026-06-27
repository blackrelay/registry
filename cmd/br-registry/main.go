package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/blackrelay/registry/internal/api"
	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/auth"
	"github.com/blackrelay/registry/internal/config"
	"github.com/blackrelay/registry/internal/db"
)

func main() {
	cfg := config.Load()
	addr := flag.String("addr", cfg.Address, "HTTP listen address")
	databaseURL := flag.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flag.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	adminToken := flag.String("admin-token", cfg.AdminToken, "local admin bearer token")
	accessTeamDomain := flag.String("access-team-domain", cfg.AccessTeamDomain, "Cloudflare Access team domain for admin auth")
	accessAudience := flag.String("access-aud", cfg.AccessAudience, "Cloudflare Access application AUD tag for admin auth")
	accessCertsURL := flag.String("access-certs-url", cfg.AccessCertsURL, "optional Cloudflare Access JWKS URL override")
	registryID := flag.String("registry-id", cfg.InstanceID, "registry instance id emitted in API response metadata")
	apiVersion := flag.String("api-version", cfg.APIVersion, "API version emitted in API response metadata")
	migrate := flag.Bool("migrate", true, "apply PostgreSQL migrations before starting")
	flag.Parse()

	runtimeConfig := config.Config{
		Address:          *addr,
		DatabaseURL:      *databaseURL,
		ArtefactRoot:     *artefactRoot,
		AdminToken:       *adminToken,
		AccessTeamDomain: *accessTeamDomain,
		AccessAudience:   *accessAudience,
		AccessCertsURL:   *accessCertsURL,
		InstanceID:       *registryID,
		APIVersion:       *apiVersion,
	}
	if err := runtimeConfig.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, runtimeConfig.DatabaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if *migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	server := api.Server{
		Store:           db.PostgresStore{Pool: pool},
		ArtefactStore:   artefacts.LocalStore{Root: runtimeConfig.ArtefactRoot},
		AdminToken:      runtimeConfig.AdminToken,
		AdminAuthorizer: adminAuthorizerFromConfig(runtimeConfig),
		RegistryID:      runtimeConfig.InstanceID,
		APIVersion:      runtimeConfig.APIVersion,
		Logger:          slog.Default(),
	}
	httpServer := &http.Server{
		Addr:              runtimeConfig.Address,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("Registry API listening", "addr", runtimeConfig.Address, "registry_id", runtimeConfig.InstanceID)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("serve API", "error", err)
		os.Exit(1)
	}
}

func adminAuthorizerFromConfig(cfg config.Config) api.AdminAuthorizer {
	if cfg.AccessTeamDomain == "" && cfg.AccessAudience == "" && cfg.AccessCertsURL == "" {
		return nil
	}
	return &auth.CloudflareAccessValidator{
		TeamDomain: cfg.AccessTeamDomain,
		Audience:   cfg.AccessAudience,
		CertsURL:   cfg.AccessCertsURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}
