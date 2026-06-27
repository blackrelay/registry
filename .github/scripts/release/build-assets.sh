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

rm -f "${out_dir}"/*

for platform in "${platforms[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  name="blackrelay-registry_${version}_${goos}_${goarch}"
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

  if [ "${goos}" = "windows" ]; then
    (
      cd "${work_dir}"
      zip -qr "${out_dir}/${name}.zip" "${name}"
    )
  else
    tar -czf "${out_dir}/${name}.tar.gz" -C "${work_dir}" "${name}"
  fi
done

(
  cd "${out_dir}"
  sha256sum *.tar.gz *.zip > SHA2-256SUMS
  sha512sum *.tar.gz *.zip > SHA2-512SUMS
)
