#!/usr/bin/env sh
set -eu

DATABASE_URL="${DATABASE_URL:-postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable}"
RUNTIME="${BR_REGISTRY_CONTAINER_RUNTIME:-auto}"

compose() {
  case "$runtime_kind" in
    docker)
      docker compose "$@"
      ;;
    podman)
      podman compose "$@"
      ;;
    podman-compose)
      podman-compose "$@"
      ;;
    *)
      echo "No Compose runtime is configured" >&2
      exit 1
      ;;
  esac
}

runtime_kind="$RUNTIME"
if [ "$RUNTIME" = "auto" ]; then
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    runtime_kind="docker"
  elif command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
    runtime_kind="podman"
  elif command -v podman-compose >/dev/null 2>&1 && podman-compose version >/dev/null 2>&1; then
    runtime_kind="podman-compose"
  else
    runtime_kind="external"
  fi
elif [ "$RUNTIME" = "docker" ]; then
  if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
    echo "Docker Compose is not available. Install Docker or rerun with BR_REGISTRY_CONTAINER_RUNTIME=podman or external." >&2
    exit 1
  fi
elif [ "$RUNTIME" = "podman" ]; then
  if command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
    runtime_kind="podman"
  elif command -v podman-compose >/dev/null 2>&1 && podman-compose version >/dev/null 2>&1; then
    runtime_kind="podman-compose"
  else
    echo "Podman Compose is not available. Install Podman/podman-compose or rerun with BR_REGISTRY_CONTAINER_RUNTIME=docker or external." >&2
    exit 1
  fi
elif [ "$RUNTIME" != "external" ]; then
  echo "Unsupported BR_REGISTRY_CONTAINER_RUNTIME: $RUNTIME" >&2
  exit 1
fi

if [ "$runtime_kind" = "external" ]; then
  echo "Using external PostgreSQL via DATABASE_URL."
else
  echo "Using $runtime_kind Compose for local PostgreSQL."
  compose up -d postgres

  i=0
  until compose exec -T postgres pg_isready -U blackrelay -d blackrelay_registry >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -gt 60 ]; then
      echo "PostgreSQL did not become ready" >&2
      exit 1
    fi
    sleep 1
  done
fi

go run ./cmd/br-migrate -database-url "$DATABASE_URL"
go run ./cmd/br-indexer -mode audit-stillness -database-url "$DATABASE_URL" -manifest testdata/fixtures/sui-packages.stillness.json
go run ./cmd/br-import static-enemies -database-url "$DATABASE_URL" -path testdata/fixtures/static-enemies.reviewed.json
go run ./cmd/br-import static-client-recipes -database-url "$DATABASE_URL" -path testdata/fixtures/static-client-recipes.reviewed.json
go run ./cmd/br-import killmail-fixture -database-url "$DATABASE_URL" -path testdata/fixtures/killmail.npc-caird.json

export_dir="$(mktemp -d)"
publish_root="$(mktemp -d)"
api_pid=""
cleanup() {
  if [ -n "$api_pid" ]; then
    kill "$api_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$export_dir" "$publish_root"
}
trap cleanup EXIT

go run ./cmd/br-export -database-url "$DATABASE_URL" -out "$export_dir"
go run ./cmd/br-export verify -dir "$export_dir"
go run ./cmd/br-export publish-local -dir "$export_dir" -root "$publish_root" -prefix registry

go run ./cmd/br-registry -database-url "$DATABASE_URL" -addr 127.0.0.1:8080 &
api_pid=$!

i=0
until curl -fsS http://127.0.0.1:8080/v1/ready >/dev/null; do
  i=$((i + 1))
  if [ "$i" -gt 60 ]; then
    echo "Registry API did not become ready" >&2
    exit 1
  fi
  sleep 1
done

curl -fsS http://127.0.0.1:8080/v1/health >/dev/null
curl -fsS 'http://127.0.0.1:8080/v1/current/characters?environment=stillness' >/dev/null
curl -fsS 'http://127.0.0.1:8080/v1/current/route-edges?environment=stillness' >/dev/null
curl -fsS 'http://127.0.0.1:8080/v1/current/recipes?environment=stillness' >/dev/null
curl -fsS 'http://127.0.0.1:8080/v1/current/blueprints?environment=stillness' >/dev/null
curl -fsS http://127.0.0.1:8080/v1/ops/freshness >/dev/null
curl -fsS http://127.0.0.1:8080/v1/ops/cursors >/dev/null
curl -fsS http://127.0.0.1:8080/v1/ops/sui-coverage >/dev/null
curl -fsS 'http://127.0.0.1:8080/v1/ops/source-gaps?environment=stillness' >/dev/null
curl -fsS http://127.0.0.1:8080/v1/killmails/killmail:stillness:fixture:caird | grep 'Caird \[NPC\]' >/dev/null
