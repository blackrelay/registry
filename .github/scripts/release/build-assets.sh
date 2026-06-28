#!/usr/bin/env bash
set -euo pipefail

version="${1:?version is required}"
out_dir="${2:-dist/release-assets}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

mkdir -p "${out_dir}"
out_dir="$(cd "${out_dir}" && pwd)"

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT

commands=(
  br-export
  br-import
  br-indexer
  br-migrate
  br-registry
)

platforms=(
  linux/amd64
  linux/arm64
  windows/amd64
  windows/arm64
  darwin/amd64
  darwin/arm64
  freebsd/amd64
  openbsd/amd64
  netbsd/amd64
)

write_zip() {
  local parent_dir="$1"
  local source_name="$2"
  local destination="$3"

  if command -v zip >/dev/null 2>&1; then
    (
      cd "${parent_dir}"
      zip -qr "${destination}" "${source_name}"
    )
    return
  fi

  local python_cmd=""
  if command -v python3 >/dev/null 2>&1; then
    python_cmd="python3"
  elif command -v python >/dev/null 2>&1; then
    python_cmd="python"
  else
    echo "zip or Python is required to create Windows release archives" >&2
    exit 1
  fi

  "${python_cmd}" - "${parent_dir}" "${source_name}" "${destination}" <<'PY'
import pathlib
import sys
import zipfile

parent = pathlib.Path(sys.argv[1])
source_name = sys.argv[2]
destination = pathlib.Path(sys.argv[3])
source = parent / source_name

with zipfile.ZipFile(destination, "w", compression=zipfile.ZIP_DEFLATED) as archive:
    for path in sorted(source.rglob("*")):
        if path.is_file():
            archive.write(path, path.relative_to(parent).as_posix())
PY
}

rm -f "${out_dir}"/*

for platform in "${platforms[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  name="blackrelay-registry_${goos}_${goarch}"
  stage="${work_dir}/${name}"
  suffix=""

  if [ "${goos}" = "windows" ]; then
    suffix=".exe"
  fi

  mkdir -p "${stage}/contracts" "${stage}/openapi"

  for command_name in "${commands[@]}"; do
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
      go build \
        -trimpath \
        -buildvcs=false \
        -ldflags="-s -w" \
        -o "${stage}/${command_name}${suffix}" \
        "./cmd/${command_name}"
  done

  cp README.md "${stage}/README.md"
  cp LICENSE "${stage}/LICENSE"
  cp openapi/registry.v1.yaml "${stage}/openapi/registry.v1.yaml"
  cp contracts/*.json "${stage}/contracts/"

  case "${goos}" in
    linux|darwin)
      cp "${stage}/br-registry" "${out_dir}/br-registry_${goos}_${goarch}"
      ;;
    windows)
      cp "${stage}/br-registry.exe" "${out_dir}/br-registry_windows_${goarch}.exe"
      ;;
  esac

  if [ "${goos}" = "windows" ]; then
    write_zip "${work_dir}" "${name}" "${out_dir}/${name}.zip"
  else
    tar -czf "${out_dir}/${name}.tar.gz" -C "${work_dir}" "${name}"
  fi
done

(
  cd "${out_dir}"
  mapfile -t checksum_files < <(
    find . -maxdepth 1 -type f \
      ! -name 'SHA2-256SUMS' \
      ! -name 'SHA2-256SUMS.sig' \
      ! -name 'SHA2-512SUMS' \
      ! -name 'SHA2-512SUMS.sig' \
      ! -name 'public.key' \
      -printf '%P\n' | sort
  )

  if [ "${#checksum_files[@]}" -eq 0 ]; then
    echo "No release assets found for checksum manifests" >&2
    exit 1
  fi

  sha256sum "${checksum_files[@]}" > SHA2-256SUMS
  sha512sum "${checksum_files[@]}" > SHA2-512SUMS
)
