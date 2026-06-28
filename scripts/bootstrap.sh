#!/usr/bin/env sh
set -eu

runtime="auto"
database_url="${DATABASE_URL:-postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable}"
client_path=""
static_universe_path=""

usage() {
  cat <<'EOF'
Usage: ./scripts/bootstrap.sh [options]

Options:
  --runtime auto|docker|podman|external
  --database-url URL
  --client-path PATH
  --static-universe-path PATH
  -h, --help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --runtime)
      runtime="${2:?--runtime requires a value}"
      shift 2
      ;;
    --database-url)
      database_url="${2:?--database-url requires a value}"
      shift 2
      ;;
    --client-path)
      client_path="${2:?--client-path requires a value}"
      shift 2
      ;;
    --static-universe-path)
      static_universe_path="${2:?--static-universe-path requires a value}"
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

case "$runtime" in
  auto|docker|podman|external) ;;
  *)
    echo "Unsupported runtime: $runtime" >&2
    exit 2
    ;;
esac

export DATABASE_URL="$database_url"
export BR_REGISTRY_CONTAINER_RUNTIME="$runtime"

./scripts/smoke.sh

if [ -n "$client_path" ]; then
  mkdir -p tmp
  if [ -z "$static_universe_path" ]; then
    static_universe_path="./tmp/static-client-universe-stillness"
  fi
  go run ./cmd/br-import static-client-decode-universe -client-path "$client_path" -out "$static_universe_path"
  go run ./cmd/br-import static-client-extract-production -client-path "$client_path" -out ./tmp/static-client-production-resources.json
  go run ./cmd/br-import static-client-extract-types -client-path "$client_path" -out ./tmp/static-client-types.probes.json
fi

if [ -n "$static_universe_path" ]; then
  go run ./cmd/br-import static-universe -database-url "$database_url" -path "$static_universe_path"
fi

./scripts/api-proof.sh --start-server --database-url "$database_url"
