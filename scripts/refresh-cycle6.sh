#!/usr/bin/env sh
set -eu

DATABASE_URL="${DATABASE_URL:-postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable}"
MANIFEST="${MANIFEST:-testdata/fixtures/sui-packages.stillness.json}"
WORLD_PACKAGE="${WORLD_PACKAGE:-0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1}"
TOKEN_PACKAGE="${TOKEN_PACKAGE:-0xac361aa5ceb726bd974f885c9dea9e55dc9bc98fa1f5731c5965a810707bf0b8}"
CONCURRENCY="${CONCURRENCY:-64}"
MAX_PAGES="${MAX_PAGES:-5}"
INCLUDE_TOKEN_PACKAGE="${INCLUDE_TOKEN_PACKAGE:-0}"
ONLY_INCOMPLETE="${ONLY_INCOMPLETE:-0}"
EXPORT_OUT="${EXPORT_OUT:-exports/cycle6-latest}"
EXPORT_CYCLES="${EXPORT_CYCLES:-current}"
EXPORT_LIMIT="${EXPORT_LIMIT:-0}"
INCLUDE_RAW_EXPORTS="${INCLUDE_RAW_EXPORTS:-0}"
SKIP_EXPORT="${SKIP_EXPORT:-0}"
PUBLISH_LOCAL="${PUBLISH_LOCAL:-0}"
PUBLISH_ROOT="${PUBLISH_ROOT:-published-exports}"
PUBLISH_PREFIX="${PUBLISH_PREFIX:-registry}"
PUBLISH_R2="${PUBLISH_R2:-0}"
SUMMARY_PATH="${SUMMARY_PATH:-tmp/cycle6-refresh-summary.json}"

if [ "${FULL:-0}" = "1" ]; then
  MAX_PAGES=0
fi

usage() {
  cat <<'EOF'
Usage: ./scripts/refresh-cycle6.sh [options]

Options:
  --database-url URL
  --manifest PATH
  --world-package PACKAGE_ID
  --token-package PACKAGE_ID
  --concurrency N
  --max-pages N
  --full
  --only-incomplete
  --include-token-package
  --export-out PATH
  --export-cycles current|all|LIST
  --export-limit N
  --include-raw-exports
  --skip-export
  --publish-local
  --publish-root PATH
  --publish-prefix PREFIX
  --publish-r2
  --summary-path PATH
  -h, --help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --database-url)
      DATABASE_URL="${2:?--database-url requires a value}"
      shift 2
      ;;
    --manifest)
      MANIFEST="${2:?--manifest requires a value}"
      shift 2
      ;;
    --world-package)
      WORLD_PACKAGE="${2:?--world-package requires a value}"
      shift 2
      ;;
    --token-package)
      TOKEN_PACKAGE="${2:?--token-package requires a value}"
      shift 2
      ;;
    --concurrency)
      CONCURRENCY="${2:?--concurrency requires a value}"
      shift 2
      ;;
    --max-pages)
      MAX_PAGES="${2:?--max-pages requires a value}"
      shift 2
      ;;
    --full)
      FULL=1
      MAX_PAGES=0
      shift
      ;;
    --only-incomplete)
      ONLY_INCOMPLETE=1
      shift
      ;;
    --include-token-package)
      INCLUDE_TOKEN_PACKAGE=1
      shift
      ;;
    --export-out)
      EXPORT_OUT="${2:?--export-out requires a value}"
      shift 2
      ;;
    --export-cycles)
      EXPORT_CYCLES="${2:?--export-cycles requires a value}"
      shift 2
      ;;
    --export-limit)
      EXPORT_LIMIT="${2:?--export-limit requires a value}"
      shift 2
      ;;
    --include-raw-exports)
      INCLUDE_RAW_EXPORTS=1
      shift
      ;;
    --skip-export)
      SKIP_EXPORT=1
      shift
      ;;
    --publish-local)
      PUBLISH_LOCAL=1
      shift
      ;;
    --publish-root)
      PUBLISH_ROOT="${2:?--publish-root requires a value}"
      shift 2
      ;;
    --publish-prefix)
      PUBLISH_PREFIX="${2:?--publish-prefix requires a value}"
      shift 2
      ;;
    --publish-r2)
      PUBLISH_R2=1
      shift
      ;;
    --summary-path)
      SUMMARY_PATH="${2:?--summary-path requires a value}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

run() {
  printf '+ %s\n' "$(redact_args "$@")"
  "$@"
}

run_capture() {
  output_path="$1"
  shift
  printf '+ %s\n' "$(redact_args "$@")"
  "$@" | tee "$output_path"
}

redact_args() {
  out=""
  redact_next=0
  for arg in "$@"; do
    if [ "$redact_next" = "1" ]; then
      arg="<redacted>"
      redact_next=0
    fi
    if [ -z "$out" ]; then
      out="$arg"
    else
      out="$out $arg"
    fi
    case "$arg" in
      -database-url|-secret-access-key|-access-key-id)
        redact_next=1
        ;;
    esac
  done
  printf '%s' "$out"
}

packages="$WORLD_PACKAGE"
if [ "$INCLUDE_TOKEN_PACKAGE" = "1" ]; then
  packages="$packages $TOKEN_PACKAGE"
fi

echo "Cycle 6 refresh mode: max-pages=$MAX_PAGES concurrency=$CONCURRENCY packages=$packages export-cycles=$EXPORT_CYCLES"
started_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
summary_dir="$(dirname "$SUMMARY_PATH")"
mkdir -p "$summary_dir"

run go run ./cmd/br-migrate -database-url "$DATABASE_URL"

for package_id in $packages; do
  run go run ./cmd/br-indexer -mode plan -database-url "$DATABASE_URL" -manifest "$MANIFEST" -package "$package_id" -cycles current -max-pages "$MAX_PAGES" -concurrency "$CONCURRENCY"

  incomplete_args=""
  if [ "$ONLY_INCOMPLETE" = "1" ]; then
    incomplete_args="-only-incomplete"
  fi

  # shellcheck disable=SC2086
  run go run ./cmd/br-indexer -mode events -database-url "$DATABASE_URL" -manifest "$MANIFEST" -package "$package_id" -cycles current -max-pages "$MAX_PAGES" -concurrency "$CONCURRENCY" -retries 12 $incomplete_args
  if [ "$package_id" = "$TOKEN_PACKAGE" ]; then
    echo "Skipping object backfill for token package $package_id; no object types are configured."
    continue
  fi
  # shellcheck disable=SC2086
  run go run ./cmd/br-indexer -mode objects -database-url "$DATABASE_URL" -manifest "$MANIFEST" -package "$package_id" -cycles current -max-pages "$MAX_PAGES" -concurrency "$CONCURRENCY" -retries 12 -allow-object-target-errors $incomplete_args
done

run go run ./cmd/br-indexer -mode derive-events -database-url "$DATABASE_URL" -cycles current -module killmail,character,gate,assembly,storage_unit,turret -derive-batch-size 5000
run go run ./cmd/br-indexer -mode derive-objects -database-url "$DATABASE_URL" -cycles current -derive-batch-size 5000
run go run ./cmd/br-indexer -mode resolve-evidence -database-url "$DATABASE_URL"
coverage_path="$SUMMARY_PATH.coverage.json"
killmail_path="$SUMMARY_PATH.killmails.json"
current_state_path="$SUMMARY_PATH.current-state.json"
report_path="$SUMMARY_PATH.report.json"
run_capture "$coverage_path" go run ./cmd/br-indexer -mode audit-stillness -database-url "$DATABASE_URL" -manifest "$MANIFEST" -package "$WORLD_PACKAGE" -cycles current
run_capture "$killmail_path" go run ./cmd/br-indexer -mode audit-killmails -database-url "$DATABASE_URL" -exclude-fixtures -sample-limit 20
run_capture "$current_state_path" go run ./cmd/br-indexer -mode audit-current-state -database-url "$DATABASE_URL"
run_capture "$report_path" go run ./cmd/br-indexer -mode report -database-url "$DATABASE_URL" -exclude-fixtures

export_path=""
verify_path=""
publish_local_path=""
publish_r2_path=""
if [ "$SKIP_EXPORT" != "1" ]; then
  export_path="$SUMMARY_PATH.export.json"
  verify_path="$SUMMARY_PATH.export-verify.json"
  export_args=""
  if [ "$INCLUDE_RAW_EXPORTS" = "1" ]; then
    export_args="-include-events -include-sui-objects -timeout 30m"
  fi
  # shellcheck disable=SC2086
  run_capture "$export_path" go run ./cmd/br-export -database-url "$DATABASE_URL" -out "$EXPORT_OUT" -cycles "$EXPORT_CYCLES" -limit "$EXPORT_LIMIT" $export_args
  run_capture "$verify_path" go run ./cmd/br-export verify -dir "$EXPORT_OUT"
  if [ "$PUBLISH_LOCAL" = "1" ]; then
    publish_local_path="$SUMMARY_PATH.publish-local.json"
    run_capture "$publish_local_path" go run ./cmd/br-export publish-local -dir "$EXPORT_OUT" -root "$PUBLISH_ROOT" -prefix "$PUBLISH_PREFIX"
  fi
  if [ "$PUBLISH_R2" = "1" ]; then
    publish_r2_path="$SUMMARY_PATH.publish-r2.json"
    run_capture "$publish_r2_path" go run ./cmd/br-export publish-r2 -dir "$EXPORT_OUT" -prefix "$PUBLISH_PREFIX"
  fi
fi

finished_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
cat > "$SUMMARY_PATH" <<EOF
{
  "schemaVersion": "registry.cycle6_refresh.v1",
  "startedAt": "$started_at",
  "finishedAt": "$finished_at",
  "manifest": "$MANIFEST",
  "worldPackage": "$WORLD_PACKAGE",
  "tokenPackage": "$TOKEN_PACKAGE",
  "packages": "$(printf '%s' "$packages")",
  "maxPages": $MAX_PAGES,
  "concurrency": $CONCURRENCY,
  "full": $(if [ "${FULL:-0}" = "1" ]; then echo true; else echo false; fi),
  "onlyIncomplete": $(if [ "$ONLY_INCOMPLETE" = "1" ]; then echo true; else echo false; fi),
  "includeTokenPackage": $(if [ "$INCLUDE_TOKEN_PACKAGE" = "1" ]; then echo true; else echo false; fi),
  "coveragePath": "$coverage_path",
  "killmailAuditPath": "$killmail_path",
  "currentStateAuditPath": "$current_state_path",
  "reportPath": "$report_path",
  "export": {
    "skipped": $(if [ "$SKIP_EXPORT" = "1" ]; then echo true; else echo false; fi),
    "out": "$EXPORT_OUT",
    "cycles": "$EXPORT_CYCLES",
    "limit": $EXPORT_LIMIT,
    "includeRaw": $(if [ "$INCLUDE_RAW_EXPORTS" = "1" ]; then echo true; else echo false; fi),
    "resultPath": "$export_path",
    "verificationPath": "$verify_path",
    "publishLocalPath": "$publish_local_path",
    "publishR2Path": "$publish_r2_path"
  }
}
EOF
echo "Cycle 6 refresh summary written to $SUMMARY_PATH"
cat "$SUMMARY_PATH"
