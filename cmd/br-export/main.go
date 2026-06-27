package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/blackrelay/registry/internal/config"
	cyclepkg "github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/exporter"
	"github.com/blackrelay/registry/internal/publisher"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "verify" {
		verifyExport(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "publish-local" {
		publishLocal(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "publish-r2" {
		publishR2(os.Args[2:])
		return
	}
	export(os.Args[1:])
}

func export(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("br-export", flag.ExitOnError)
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	limit := flags.Int("limit", 0, "maximum rows per collection to export; 0 exports all available rows")
	outputDir := flags.String("out", "exports", "public export output directory")
	includeEvents := flags.Bool("include-events", false, "include raw indexed events.jsonl")
	includeSuiObjects := flags.Bool("include-sui-objects", false, "include raw indexed sui_objects.jsonl")
	cycleScopeValue := flags.String("cycles", "", "cycle scope: current, all, or comma-separated cycle numbers such as 5,6; default is current plus unlabelled rows")
	registryID := flags.String("registry-id", cfg.InstanceID, "registry instance id emitted in catalog and manifest metadata")
	apiVersion := flags.String("api-version", cfg.APIVersion, "API version emitted in catalog and manifest metadata")
	timeout := flags.Duration("timeout", 10*time.Minute, "export timeout")
	flags.Parse(args)
	cycleScope, err := cyclepkg.ParseScope(*cycleScopeValue, true)
	if err != nil {
		slog.Error("parse cycles", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := db.Connect(ctx, *databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	store := db.PostgresStore{Pool: pool}
	result, err := exporter.WritePublicExport(ctx, store, *outputDir, exporter.ExportOptions{
		Limit:             *limit,
		IncludeEvents:     *includeEvents,
		IncludeSuiObjects: *includeSuiObjects,
		CycleScope:        exportCycleScopeLabel(*cycleScopeValue),
		Cycles:            cycleScope.Cycles,
		IncludeUncycled:   cycleScope.IncludeUncycled,
		RegistryID:        *registryID,
		APIVersion:        *apiVersion,
	})
	if err != nil {
		slog.Error("write public export", "error", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		slog.Error("write export", "error", err)
		os.Exit(1)
	}
}

func verifyExport(args []string) {
	flags := flag.NewFlagSet("br-export verify", flag.ExitOnError)
	dir := flags.String("dir", "exports", "export directory containing manifest.json")
	flags.Parse(args)
	result, err := exporter.VerifyPublicExport(*dir)
	if err != nil {
		slog.Error("verify public export", "error", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		slog.Error("write verification result", "error", err)
		os.Exit(1)
	}
	if !result.Valid {
		os.Exit(1)
	}
}

func publishLocal(args []string) {
	flags := flag.NewFlagSet("br-export publish-local", flag.ExitOnError)
	dir := flags.String("dir", "exports", "verified export directory")
	root := flags.String("root", "published-exports", "local publish root")
	prefix := flags.String("prefix", "registry/current", "object key prefix")
	bundleID := flags.String("bundle-id", "", "optional immutable bundle id; defaults to manifest SHA-256")
	timeout := flags.Duration("timeout", time.Minute, "publish timeout")
	flags.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := publisher.PublishVerifiedExport(ctx, *dir, publisher.LocalStore{Root: *root}, publisher.Options{
		Prefix:   *prefix,
		BundleID: *bundleID,
	})
	if err != nil {
		slog.Error("publish local export", "error", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		slog.Error("write publish result", "error", err)
		os.Exit(1)
	}
}

func publishR2(args []string) {
	flags := flag.NewFlagSet("br-export publish-r2", flag.ExitOnError)
	dir := flags.String("dir", "exports", "verified export directory")
	accountID := flags.String("account-id", os.Getenv("BR_R2_ACCOUNT_ID"), "Cloudflare account id; used to build the R2 endpoint")
	endpoint := flags.String("endpoint", os.Getenv("BR_R2_ENDPOINT"), "S3-compatible endpoint; defaults to https://<account-id>.r2.cloudflarestorage.com")
	accessKeyID := flags.String("access-key-id", os.Getenv("BR_R2_ACCESS_KEY_ID"), "R2 access key id")
	secretAccessKey := flags.String("secret-access-key", os.Getenv("BR_R2_SECRET_ACCESS_KEY"), "R2 secret access key")
	bucket := flags.String("bucket", os.Getenv("BR_R2_BUCKET"), "R2 bucket name")
	region := flags.String("region", envOr("BR_R2_REGION", "auto"), "S3 signing region")
	prefix := flags.String("prefix", "registry/current", "object key prefix")
	bundleID := flags.String("bundle-id", "", "optional immutable bundle id; defaults to manifest SHA-256")
	timeout := flags.Duration("timeout", 5*time.Minute, "publish timeout")
	flags.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	store, err := publisher.NewR2Store(ctx, publisher.R2Options{
		AccountID:       *accountID,
		Endpoint:        *endpoint,
		AccessKeyID:     *accessKeyID,
		SecretAccessKey: *secretAccessKey,
		Bucket:          *bucket,
		Region:          *region,
	})
	if err != nil {
		slog.Error("configure R2 publisher", "error", err)
		os.Exit(1)
	}
	result, err := publisher.PublishVerifiedExport(ctx, *dir, store, publisher.Options{
		Prefix:   *prefix,
		BundleID: *bundleID,
	})
	if err != nil {
		slog.Error("publish R2 export", "error", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		slog.Error("write publish result", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func exportCycleScopeLabel(value string) string {
	if value == "" {
		return "current"
	}
	return value
}
