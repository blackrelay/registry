#!/usr/bin/env sh
set -eu

base_url="http://127.0.0.1:8080"
addr="127.0.0.1:8080"
database_url="${DATABASE_URL:-postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable}"
start_server=0
timeout_seconds=30

usage() {
  cat <<'EOF'
Usage: ./scripts/api-proof.sh [options]

Options:
  --base-url URL          Registry API base URL, default http://127.0.0.1:8080
  --start-server          Start br-registry for the proof run
  --addr ADDR             Listen address when --start-server is used, default 127.0.0.1:8080
  --database-url URL      PostgreSQL connection string
  --timeout-seconds N     Readiness timeout, default 30
  -h, --help              Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-url)
      base_url="${2:?--base-url requires a value}"
      shift 2
      ;;
    --start-server)
      start_server=1
      shift
      ;;
    --addr)
      addr="${2:?--addr requires a value}"
      shift 2
      ;;
    --database-url)
      database_url="${2:?--database-url requires a value}"
      shift 2
      ;;
    --timeout-seconds)
      timeout_seconds="${2:?--timeout-seconds requires a value}"
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

server_pid=""
cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

fetch() {
  path="$1"
  curl -fsS --max-time 10 "${base_url}${path}"
}

if [ "$start_server" = "1" ]; then
  if [ "$base_url" = "http://127.0.0.1:8080" ] && [ "$addr" != "127.0.0.1:8080" ]; then
    base_url="http://${addr}"
  fi
  go run ./cmd/br-registry -addr "$addr" -database-url "$database_url" &
  server_pid="$!"
fi

i=0
ready_status=""
while [ "$i" -lt "$timeout_seconds" ]; do
  if ready_json="$(fetch "/v1/ready" 2>/dev/null)"; then
    case "$ready_json" in
      *'"status":"ready"'*)
        ready_status="ready"
        break
        ;;
    esac
  fi
  i=$((i + 1))
  sleep 1
done

if [ "$ready_status" != "ready" ]; then
  echo "Registry API did not become ready within ${timeout_seconds}s" >&2
  exit 1
fi

health_json="$(fetch "/v1/health")"
case "$health_json" in
  *'"status":"ok"'*) ;;
  *)
    echo "Health endpoint returned an unexpected response" >&2
    exit 1
    ;;
esac

fetch "/v1/killmails?environment=stillness&exclude_fixtures=true&limit=1" >/dev/null
fetch "/v1/current/characters?environment=stillness&has_activity=true&limit=1" >/dev/null
fetch "/v1/current/systems?environment=stillness&has_activity=true&limit=1" >/dev/null
fetch "/v1/current/route-edges?environment=stillness&limit=1" >/dev/null
fetch "/v1/current/enemies?environment=stillness&limit=1" >/dev/null
fetch "/v1/current/materials?environment=stillness&limit=1" >/dev/null
fetch "/v1/current/recipes?environment=stillness&limit=1" >/dev/null
fetch "/v1/current/blueprints?environment=stillness&limit=1" >/dev/null
fetch "/v1/ops/sui-coverage" >/dev/null
fetch "/v1/ops/source-gaps?environment=stillness" >/dev/null

cat <<EOF
{
  "schemaVersion": "registry.api_proof.v1",
  "baseUrl": "$base_url",
  "ready": "ready",
  "health": "ok",
  "checked": [
    "/v1/killmails",
    "/v1/current/characters",
    "/v1/current/systems",
    "/v1/current/route-edges",
    "/v1/current/enemies",
    "/v1/current/materials",
    "/v1/current/recipes",
    "/v1/current/blueprints",
    "/v1/ops/sui-coverage",
    "/v1/ops/source-gaps"
  ]
}
EOF
