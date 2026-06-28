package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/config"
	cyclepkg "github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/importer"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/snapshots"
	"github.com/blackrelay/registry/internal/staticclient"
	"github.com/blackrelay/registry/internal/worldapi"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "datahub-types":
		importDatahubTypes(os.Args[2:])
	case "world-systems":
		importWorldSystems(os.Args[2:])
	case "world-tribes":
		importWorldTribes(os.Args[2:])
	case "static-universe":
		importStaticUniverse(os.Args[2:])
	case "static-client-decode-universe":
		decodeStaticClientUniverse(os.Args[2:])
	case "static-client-extract-types":
		extractStaticClientTypes(os.Args[2:])
	case "static-client-decode-types":
		decodeStaticClientTypes(os.Args[2:])
	case "static-client-compare-types":
		compareStaticClientTypes(os.Args[2:])
	case "static-client-extract-production":
		extractStaticClientProduction(os.Args[2:])
	case "static-client-decode-production":
		decodeStaticClientProduction(os.Args[2:])
	case "static-client-compare-production":
		compareStaticClientProduction(os.Args[2:])
	case "static-client-summarise-production":
		summariseStaticClientProduction(os.Args[2:])
	case "static-client-inspect-types":
		inspectStaticClientTypes(os.Args[2:])
	case "static-client-types":
		importStaticClientTypes(os.Args[2:])
	case "static-client-recipes":
		importStaticClientRecipes(os.Args[2:])
	case "static-client-enemies":
		importStaticClientEnemies(os.Args[2:])
	case "static-enemies":
		importStaticEnemies(os.Args[2:])
	case "static-enemies-jsonl":
		importStaticEnemiesJSONL(os.Args[2:])
	case "tribe-identities":
		importTribeIdentities(os.Args[2:])
	case "killmail-fixture":
		importKillmailFixture(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func inspectStaticClientTypes(args []string) {
	os.Exit(runStaticClientInspectTypes(args, os.Stdout, os.Stderr))
}

func runStaticClientInspectTypes(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-inspect-types", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientPath := flags.String("client-path", "", "installed EVE Frontier client root containing resfileindex files")
	probeTypeIDs := flags.String("probe-type-ids", "92096,92098,92271,92273,85702,88089,94167,95283,95291,95504,5033,5130", "comma-separated type or group IDs to search as little-endian uint32 probes")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*clientPath) == "" {
		fmt.Fprintln(stderr, "-client-path is required")
		return 2
	}
	probes, err := parseIntList(*probeTypeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "parse probe type ids: %v\n", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := staticclient.InspectStaticClientTypes(ctx, staticclient.StaticTypeInspectionOptions{
		ClientRoot:   *clientPath,
		ProbeTypeIDs: probes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "inspect static-client types: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "parser status: %s\n", result.ParserStatus)
	fmt.Fprintf(stdout, "types.fsdbinary path: %s\n", result.TypeFile.Path)
	fmt.Fprintf(stdout, "types.fsdbinary sha256: %s\n", result.TypeResource.SHA256)
	fmt.Fprintf(stdout, "types.fsdbinary size bytes: %d\n", result.TypeFile.SizeBytes)
	fmt.Fprintf(stdout, "types.fsdbinary header hex: %s\n", result.TypeFile.HeaderHex)
	if result.LocalizationResource != nil {
		fmt.Fprintf(stdout, "localisation sha256: %s\n", result.LocalizationResource.SHA256)
	}
	if discoveries, err := staticclient.DiscoverStaticClientResources(*clientPath); err == nil {
		for _, item := range discoveries {
			fmt.Fprintf(stdout, "resource candidate: %s %s %s\n", item.Kind, item.ResourcePath, item.Evidence.SHA256)
		}
	}
	for _, probe := range result.Probes {
		offsets := make([]string, 0, len(probe.LittleEndianOffsets))
		for _, offset := range probe.LittleEndianOffsets {
			offsets = append(offsets, strconv.FormatInt(offset, 10))
		}
		fmt.Fprintf(stdout, "probe type %d offsets: %s\n", probe.TypeID, strings.Join(offsets, ","))
	}
	for _, row := range result.DecodedRows {
		name := row.Name
		if name == "" {
			name = "(unresolved)"
		}
		fmt.Fprintf(stdout, "decoded type row: type=%d group=%d typeName=%d name=%q wreck=%d offset=%d\n", row.TypeID, row.GroupID, row.TypeNameID, name, row.WreckTypeID, row.OffsetBytes)
	}
	return 0
}

func extractStaticClientTypes(args []string) {
	os.Exit(runStaticClientExtractTypes(args, os.Stdout, os.Stderr))
}

func decodeStaticClientTypes(args []string) {
	os.Exit(runStaticClientDecodeTypes(args, os.Stdout, os.Stderr))
}

func decodeStaticClientUniverse(args []string) {
	os.Exit(runStaticClientDecodeUniverse(args, os.Stdout, os.Stderr))
}

func runStaticClientDecodeUniverse(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-decode-universe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientPath := flags.String("client-path", "", "installed EVE Frontier client root containing resfileindex files")
	outputDir := flags.String("out", "", "output static-universe extraction directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*clientPath) == "" {
		fmt.Fprintln(stderr, "-client-path is required")
		return 2
	}
	if strings.TrimSpace(*outputDir) == "" {
		fmt.Fprintln(stderr, "-out is required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := staticclient.DecodeStaticClientUniverseFiles(ctx, staticclient.StaticUniverseDecodeOptions{
		ClientRoot:  *clientPath,
		OutputDir:   *outputDir,
		Environment: model.Environment(*environment),
		ClientBuild: *clientBuild,
		PatchLabel:  *patchLabel,
	})
	if err != nil {
		fmt.Fprintf(stderr, "decode static-client universe rows: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func compareStaticClientTypes(args []string) {
	os.Exit(runStaticClientCompareTypes(args, os.Stdout, os.Stderr))
}

func runStaticClientCompareTypes(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-compare-types", flag.ContinueOnError)
	flags.SetOutput(stderr)
	resolvedPath := flags.String("resolved", "", "reviewed or resolved static-client type JSON path")
	nativePath := flags.String("native", "", "native static-client type JSON path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*resolvedPath) == "" {
		fmt.Fprintln(stderr, "-resolved is required")
		return 2
	}
	if strings.TrimSpace(*nativePath) == "" {
		fmt.Fprintln(stderr, "-native is required")
		return 2
	}
	result, err := staticclient.CompareStaticTypeFiles(*resolvedPath, *nativePath)
	if err != nil {
		fmt.Fprintf(stderr, "compare static-client types: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func compareStaticClientProduction(args []string) {
	os.Exit(runStaticClientCompareProduction(args, os.Stdout, os.Stderr))
}

func decodeStaticClientProduction(args []string) {
	os.Exit(runStaticClientDecodeProduction(args, os.Stdout, os.Stderr))
}

func summariseStaticClientProduction(args []string) {
	os.Exit(runStaticClientSummariseProduction(args, os.Stdout, os.Stderr))
}

func runStaticClientDecodeProduction(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-decode-production", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientPath := flags.String("client-path", "", "installed EVE Frontier client root containing resfileindex files")
	outputPath := flags.String("out", "", "optional output Registry static-client production decode JSON artefact path")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*clientPath) == "" {
		fmt.Fprintln(stderr, "-client-path is required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := staticclient.DecodeStaticClientProductionFiles(ctx, staticclient.StaticProductionDecodeOptions{
		ClientRoot:  *clientPath,
		OutputPath:  *outputPath,
		Environment: model.Environment(*environment),
		ClientBuild: *clientBuild,
		PatchLabel:  *patchLabel,
	})
	if err != nil {
		fmt.Fprintf(stderr, "decode static-client production rows: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func runStaticClientCompareProduction(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-compare-production", flag.ContinueOnError)
	flags.SetOutput(stderr)
	beforePath := flags.String("before", "", "previous static-client production resource manifest path")
	afterPath := flags.String("after", "", "candidate static-client production resource manifest path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*beforePath) == "" {
		fmt.Fprintln(stderr, "-before is required")
		return 2
	}
	if strings.TrimSpace(*afterPath) == "" {
		fmt.Fprintln(stderr, "-after is required")
		return 2
	}
	result, err := staticclient.CompareStaticProductionResourceFiles(*beforePath, *afterPath)
	if err != nil {
		fmt.Fprintf(stderr, "compare static-client production resources: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func runStaticClientSummariseProduction(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-summarise-production", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("path", "", "static-client production resource manifest path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*path) == "" {
		fmt.Fprintln(stderr, "-path is required")
		return 2
	}
	result, err := staticclient.SummariseStaticProductionResourceFile(*path)
	if err != nil {
		fmt.Fprintf(stderr, "summarise static-client production resources: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func runStaticClientExtractTypes(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-extract-types", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientPath := flags.String("client-path", "", "installed EVE Frontier client root containing resfileindex files")
	resolvedJSON := flags.String("resolved-json", "", "resolved static-client type JSON path")
	probeTypeIDs := flags.String("probe-type-ids", "92096,92098,92271,92273,85702,88089,94167,95283,95291,95504", "comma-separated static type IDs to decode from native type-row probes when -resolved-json is omitted")
	nativeScan := flags.Bool("native-scan", false, "scan types.fsdbinary directly for localisation-backed type rows when -resolved-json is omitted")
	outputPath := flags.String("out", "", "output Registry static-client type JSON artefact path")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*clientPath) == "" {
		fmt.Fprintln(stderr, "-client-path is required")
		return 2
	}
	if strings.TrimSpace(*outputPath) == "" {
		fmt.Fprintln(stderr, "-out is required")
		return 2
	}
	probes, err := parseIntList(*probeTypeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "parse probe type ids: %v\n", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := staticclient.ExtractStaticClientTypes(ctx, staticclient.StaticTypeExtractionOptions{
		ClientRoot:       *clientPath,
		ResolvedJSONPath: *resolvedJSON,
		OutputPath:       *outputPath,
		Environment:      model.Environment(*environment),
		ProbeTypeIDs:     probes,
		NativeFullScan:   *nativeScan,
		ClientBuild:      *clientBuild,
		PatchLabel:       *patchLabel,
	})
	if err != nil {
		fmt.Fprintf(stderr, "extract static-client types: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func runStaticClientDecodeTypes(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("static-client-decode-types", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientPath := flags.String("client-path", "", "installed EVE Frontier client root containing resfileindex files")
	outputPath := flags.String("out", "", "optional output Registry static-client type decode JSON artefact path")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*clientPath) == "" {
		fmt.Fprintln(stderr, "-client-path is required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := staticclient.DecodeStaticClientTypeFile(ctx, staticclient.StaticTypeDecodeOptions{
		ClientRoot:  *clientPath,
		OutputPath:  *outputPath,
		Environment: model.Environment(*environment),
		ClientBuild: *clientBuild,
		PatchLabel:  *patchLabel,
	})
	if err != nil {
		fmt.Fprintf(stderr, "decode static-client types: %v\n", err)
		return 1
	}
	if err := writeJSONTo(stdout, result); err != nil {
		fmt.Fprintf(stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func extractStaticClientProduction(args []string) {
	flags := flag.NewFlagSet("static-client-extract-production", flag.ExitOnError)
	clientPath := flags.String("client-path", "", "installed EVE Frontier client root containing resfileindex files")
	outputPath := flags.String("out", "", "output Registry static-client production-resource JSON artefact path")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	flags.Parse(args)
	if strings.TrimSpace(*clientPath) == "" {
		slog.Error("extract static-client production resources", "error", "-client-path is required")
		os.Exit(2)
	}
	if strings.TrimSpace(*outputPath) == "" {
		slog.Error("extract static-client production resources", "error", "-out is required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := staticclient.ExtractStaticClientProductionResources(ctx, staticclient.StaticProductionExtractionOptions{
		ClientRoot:  *clientPath,
		OutputPath:  *outputPath,
		Environment: model.Environment(*environment),
		ClientBuild: *clientBuild,
		PatchLabel:  *patchLabel,
	})
	if err != nil {
		slog.Error("extract static-client production resources", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importStaticUniverse(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("static-universe", flag.ExitOnError)
	path := flags.String("path", "", "static-client extraction directory containing fsd_binary_schema")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	if strings.TrimSpace(*path) == "" {
		slog.Error("import static universe", "error", "-path is required")
		os.Exit(2)
	}
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := staticclient.ImportUniverse(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, staticclient.UniverseOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", "tmp", ".", *artefactRoot},
		ClientBuild:     *clientBuild,
		PatchLabel:      *patchLabel,
		Cycle:           cycle,
		Notes:           "Imported by br-import static-universe.",
	})
	if err != nil {
		slog.Error("import static-client universe", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importStaticClientTypes(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("static-client-types", flag.ExitOnError)
	path := flags.String("path", "", "static-client type JSON artefact path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	if strings.TrimSpace(*path) == "" {
		slog.Error("import static-client types", "error", "-path is required")
		os.Exit(2)
	}
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := staticclient.ImportTypes(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, staticclient.TypeImportOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", "tmp", ".", *artefactRoot},
		ClientBuild:     *clientBuild,
		PatchLabel:      *patchLabel,
		Cycle:           cycle,
		Notes:           "Imported by br-import static-client-types.",
	})
	if err != nil {
		slog.Error("import static-client types", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importStaticClientRecipes(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("static-client-recipes", flag.ExitOnError)
	path := flags.String("path", "", "static-client recipe JSON artefact path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	if strings.TrimSpace(*path) == "" {
		slog.Error("import static-client recipes", "error", "-path is required")
		os.Exit(2)
	}
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := staticclient.ImportRecipes(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, staticclient.RecipeImportOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", "tmp", ".", *artefactRoot},
		ClientBuild:     *clientBuild,
		PatchLabel:      *patchLabel,
		Cycle:           cycle,
		Notes:           "Imported by br-import static-client-recipes.",
	})
	if err != nil {
		slog.Error("import static-client recipes", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importStaticClientEnemies(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("static-client-enemies", flag.ExitOnError)
	path := flags.String("path", "", "static-client type resolver JSON path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	enemyGroups := flags.String("enemy-groups", "5033,4963,4770,5130", "comma-separated static-client group IDs treated as reviewed NPC groups")
	enemyTypeIDs := flags.String("enemy-type-ids", "85702,88089", "comma-separated reviewed individual enemy type IDs")
	wreckTypeID := flags.Int("wreck-type-id", 81610, "required wreck type ID for default enemy extraction")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	if strings.TrimSpace(*path) == "" {
		slog.Error("import static-client enemies", "error", "-path is required")
		os.Exit(2)
	}
	groups, err := parseIntList(*enemyGroups)
	if err != nil {
		slog.Error("parse enemy groups", "error", err)
		os.Exit(2)
	}
	typeIDs, err := parseIntList(*enemyTypeIDs)
	if err != nil {
		slog.Error("parse enemy type ids", "error", err)
		os.Exit(2)
	}
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := staticclient.ImportEnemyCandidates(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, staticclient.EnemyCandidateOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", "tmp", ".", *artefactRoot},
		ClientBuild:     *clientBuild,
		PatchLabel:      *patchLabel,
		Cycle:           cycle,
		Notes:           "Imported by br-import static-client-enemies.",
		EnemyGroupIDs:   groups,
		EnemyTypeIDs:    typeIDs,
		WreckTypeID:     *wreckTypeID,
	})
	if err != nil {
		slog.Error("import static-client enemies", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importDatahubTypes(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("datahub-types", flag.ExitOnError)
	path := flags.String("path", "testdata/fixtures/datahub-types.json", "Datahub type metadata JSON path")
	fetchURL := flags.String("url", "", "fetch a public Datahub JSON snapshot before importing")
	snapshotPath := flags.String("snapshot-path", "", "local output path for a fetched Datahub snapshot")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	sourceURL := flags.String("source-url", "", "public source URL recorded for provenance; private EVE Frontier hosts are rejected")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	importPath := *path
	sourceLocator := *sourceURL
	if *fetchURL != "" {
		if *snapshotPath == "" {
			slog.Error("fetch Datahub snapshot", "error", "-snapshot-path is required when -url is set")
			os.Exit(1)
		}
		if _, err := worldapi.FetchSnapshot(ctx, *fetchURL, *snapshotPath); err != nil {
			slog.Error("fetch Datahub snapshot", "error", err)
			os.Exit(1)
		}
		importPath = *snapshotPath
		sourceLocator = *fetchURL
	}
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := worldapi.ImportDatahubTypes(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, importPath, worldapi.MetadataOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", ".", *artefactRoot},
		SourceURL:       sourceLocator,
		Cycle:           cycle,
		Notes:           "Imported by br-import datahub-types.",
	})
	if err != nil {
		slog.Error("import Datahub type metadata", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importWorldSystems(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("world-systems", flag.ExitOnError)
	path := flags.String("path", "testdata/fixtures/world-systems.json", "World API solar system metadata JSON path")
	fetchURL := flags.String("url", "", "fetch a public World API JSON snapshot before importing")
	snapshotPath := flags.String("snapshot-path", "", "local output path for a fetched World API snapshot")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	sourceURL := flags.String("source-url", "", "public source URL recorded for provenance; private EVE Frontier hosts are rejected")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	importPath := *path
	sourceLocator := *sourceURL
	if *fetchURL != "" {
		if *snapshotPath == "" {
			slog.Error("fetch World API snapshot", "error", "-snapshot-path is required when -url is set")
			os.Exit(1)
		}
		if _, err := worldapi.FetchSnapshot(ctx, *fetchURL, *snapshotPath); err != nil {
			slog.Error("fetch World API snapshot", "error", err)
			os.Exit(1)
		}
		importPath = *snapshotPath
		sourceLocator = *fetchURL
	}
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := worldapi.ImportWorldSystems(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, importPath, worldapi.MetadataOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", ".", *artefactRoot},
		SourceURL:       sourceLocator,
		Cycle:           cycle,
		Notes:           "Imported by br-import world-systems.",
	})
	if err != nil {
		slog.Error("import World API solar system metadata", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importWorldTribes(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("world-tribes", flag.ExitOnError)
	path := flags.String("path", "testdata/fixtures/world-tribes.json", "World API tribe metadata JSON path")
	fetchURL := flags.String("url", "", "fetch a public World API tribe JSON snapshot before importing")
	snapshotPath := flags.String("snapshot-path", "", "local output path for a fetched World API tribe snapshot")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	sourceURL := flags.String("source-url", "", "public source URL recorded for provenance; private EVE Frontier hosts are rejected")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	importPath := *path
	sourceLocator := *sourceURL
	if *fetchURL != "" {
		if *snapshotPath == "" {
			slog.Error("fetch World API tribe snapshot", "error", "-snapshot-path is required when -url is set")
			os.Exit(1)
		}
		if _, err := worldapi.FetchSnapshot(ctx, *fetchURL, *snapshotPath); err != nil {
			slog.Error("fetch World API tribe snapshot", "error", err)
			os.Exit(1)
		}
		importPath = *snapshotPath
		sourceLocator = *fetchURL
	}
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := worldapi.ImportWorldTribes(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, importPath, worldapi.MetadataOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", ".", *artefactRoot},
		SourceURL:       sourceLocator,
		Cycle:           cycle,
		Notes:           "Imported by br-import world-tribes.",
	})
	if err != nil {
		slog.Error("import World API tribe metadata", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importStaticEnemiesJSONL(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("static-enemies-jsonl", flag.ExitOnError)
	path := flags.String("path", "testdata/fixtures/static-enemies.before.jsonl", "static enemy JSONL extraction path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	clientBuild := flags.String("client-build", "", "client build label from the extraction source")
	patchLabel := flags.String("patch-label", "", "patch label or operator label for this extraction")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := snapshots.ProcessStaticEnemyJSONL(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, snapshots.PipelineOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", ".", *artefactRoot},
		ClientBuild:     *clientBuild,
		PatchLabel:      *patchLabel,
		Cycle:           cycle,
	})
	if err != nil {
		slog.Error("process static enemy JSONL snapshot", "error", err)
		os.Exit(1)
	}
	printSnapshotResult(result)
}

func importStaticEnemies(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("static-enemies", flag.ExitOnError)
	path := flags.String("path", "testdata/fixtures/static-enemies.reviewed.json", "reviewed static enemy candidate JSON path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	cycleValue := flags.String("cycles", "current", "cycle stamp for this import: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := importer.ImportStaticEnemies(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, importer.StaticEnemyOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", "."},
		Cycle:           cycle,
		Notes:           "Imported by br-import static-enemies.",
	})
	if err != nil {
		slog.Error("import static enemies", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importTribeIdentities(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("tribe-identities", flag.ExitOnError)
	path := flags.String("path", "", "reviewed public tribe identity JSON path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	artefactRoot := flags.String("artefact-root", cfg.ArtefactRoot, "local source artefact storage directory")
	environment := flags.String("environment", string(model.EnvironmentStillness), "registry environment")
	cycleValue := flags.String("cycles", "current", "cycle stamp for rows without a cycle: current or 6")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)
	if strings.TrimSpace(*path) == "" {
		slog.Error("import tribe identities", "error", "-path is required")
		os.Exit(2)
	}
	cycle, err := singleImportCycle(*cycleValue)
	if err != nil {
		slog.Error("parse import cycles", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	result, err := importer.ImportTribeIdentities(ctx, store, artefacts.LocalStore{Root: *artefactRoot}, *path, importer.TribeIdentityOptions{
		Environment:     model.Environment(*environment),
		AllowedRootDirs: []string{"testdata", "local-extract", "tmp", ".", *artefactRoot},
		Cycle:           cycle,
		Notes:           "Imported by br-import tribe-identities.",
	})
	if err != nil {
		slog.Error("import tribe identities", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func importKillmailFixture(args []string) {
	cfg := config.Load()
	flags := flag.NewFlagSet("killmail-fixture", flag.ExitOnError)
	path := flags.String("path", "testdata/fixtures/killmail.npc-caird.json", "killmail fixture JSON path")
	databaseURL := flags.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	migrate := flags.Bool("migrate", true, "apply migrations before importing")
	flags.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(ctx, *databaseURL, *migrate)
	defer pool.Close()
	data, err := os.ReadFile(*path)
	if err != nil {
		slog.Error("read killmail fixture", "error", err)
		os.Exit(1)
	}
	var raw model.KillmailRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Error("decode killmail fixture", "error", err)
		os.Exit(1)
	}
	if err := store.UpsertKillmail(ctx, raw); err != nil {
		slog.Error("import killmail fixture", "error", err)
		os.Exit(1)
	}
	writeJSON(map[string]any{"imported": raw.ID})
}

func openStore(ctx context.Context, databaseURL string, migrate bool) (db.PostgresStore, interface{ Close() }) {
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	if migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			pool.Close()
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	return db.PostgresStore{Pool: pool}, pool
}

func writeJSON(value any) {
	if err := writeJSONTo(os.Stdout, value); err != nil {
		slog.Error("write JSON", "error", err)
		os.Exit(1)
	}
}

func writeJSONTo(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: br-import <datahub-types|world-systems|world-tribes|static-universe|static-client-decode-universe|static-client-extract-types|static-client-decode-types|static-client-compare-types|static-client-extract-production|static-client-decode-production|static-client-compare-production|static-client-summarise-production|static-client-inspect-types|static-client-types|static-client-recipes|static-client-enemies|static-enemies|static-enemies-jsonl|tribe-identities|killmail-fixture> [flags]")
}

func printSnapshotResult(result snapshots.PipelineResult) {
	diff, err := json.Marshal(result.DiffSummary)
	if err != nil {
		diff = []byte("{}")
	}
	fmt.Fprintf(os.Stdout, "artefact hash: %s\n", result.ArtefactHash)
	fmt.Fprintf(os.Stdout, "row count: %d\n", result.RowCount)
	fmt.Fprintf(os.Stdout, "byte-identical: %s\n", yesNo(result.ByteIdentical))
	fmt.Fprintf(os.Stdout, "semantically unchanged: %s\n", yesNo(result.SemanticallyUnchanged))
	fmt.Fprintf(os.Stdout, "diff summary: %s\n", diff)
	fmt.Fprintf(os.Stdout, "promoted: %s\n", yesNo(result.Promoted))
	fmt.Fprintf(os.Stdout, "outbox jobs appended: %v\n", result.OutboxJobsAppended)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func parseIntList(value string) ([]int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid positive integer %q", part)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func singleImportCycle(value string) (*int, error) {
	scope, err := cyclepkg.ParseScope(value, true)
	if err != nil {
		return nil, err
	}
	if scope.All() {
		return nil, nil
	}
	if len(scope.Cycles) != 1 {
		return nil, fmt.Errorf("import commands accept current or one supported cycle, got %s", strings.Join(intsToStrings(scope.Cycles), ","))
	}
	cycle := scope.Cycles[0]
	return &cycle, nil
}

func intsToStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}
