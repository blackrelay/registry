# Release

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/blackrelay/registry?label=Release&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/releases/latest "Latest Release")
[![Release Workflow](https://img.shields.io/github/actions/workflow/status/blackrelay/registry/release.yml?label=Release%20Workflow&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/actions/workflows/release.yml "Release Workflow")
[![SHA-256](https://img.shields.io/badge/SHA--256-signed-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](https://github.com/blackrelay/registry/releases/latest/download/SHA2-256SUMS "Signed SHA-256 Manifest")
[![Public Key](https://img.shields.io/badge/Public%20Key-release-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](https://github.com/blackrelay/registry/releases/latest/download/public.key "Release Signing Public Key")
[![Licence](https://img.shields.io/badge/Licence-Apache--2.0-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](../../LICENSE "Apache-2.0 Licence")

</div>

Black Relay Registry releases are built by `.github/workflows/release.yml`.
- [Release Notes](#release-notes)
- [Release Files](#release-files)
- [Integrity Assets](#integrity-assets)
- [Release Signing](#release-signing)
- [Release Provenance](#release-provenance)
- [Publishing](#publishing)
- [Verification](#verification)

# Release Notes

Each release has a checked-in note at `docs/release/<version>.md`.

The checked-in note:
- does not start with `# Black Relay Registry <version>` because the GitHub release title supplies that heading;
- contains editorial release content such as highlights, security notes and operator notes;
- contains `<!-- changelog: do not remove this line or add a changelog -->` where the generated changelog should begin.

The release workflow publishes a GitHub release body by:
- prepending one line of download and verification badges using Black Relay colours;
- adding a short release-file and checksum verification note;
- copying the checked-in release note until the changelog marker;
- appending a generated `## Changelog` section from Git history.

# Release Files

Primary files are direct `br-registry` server binaries:

File | Description
:--- | :---
[`br-registry_windows_amd64.exe`](https://github.com/blackrelay/registry/releases/latest/download/br-registry_windows_amd64.exe) | Windows x64 API server binary
[`br-registry_linux_amd64`](https://github.com/blackrelay/registry/releases/latest/download/br-registry_linux_amd64) | Linux x64 API server binary
[`br-registry_darwin_arm64`](https://github.com/blackrelay/registry/releases/latest/download/br-registry_darwin_arm64) | macOS Apple Silicon API server binary

Alternative direct server binaries:

File | Description
:--- | :---
[`br-registry_windows_arm64.exe`](https://github.com/blackrelay/registry/releases/latest/download/br-registry_windows_arm64.exe) | Windows Arm64 API server binary
[`br-registry_linux_arm64`](https://github.com/blackrelay/registry/releases/latest/download/br-registry_linux_arm64) | Linux Arm64 API server binary
[`br-registry_darwin_amd64`](https://github.com/blackrelay/registry/releases/latest/download/br-registry_darwin_amd64) | macOS Intel API server binary

Command bundles include all Registry commands:
- `br-export`
- `br-import`
- `br-indexer`
- `br-migrate`
- `br-registry`

Bundle | Description
:--- | :---
[`blackrelay-registry_linux_amd64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_linux_amd64.tar.gz) | Linux x64 command bundle
[`blackrelay-registry_linux_arm64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_linux_arm64.tar.gz) | Linux Arm64 command bundle
[`blackrelay-registry_windows_amd64.zip`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_windows_amd64.zip) | Windows x64 command bundle
[`blackrelay-registry_windows_arm64.zip`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_windows_arm64.zip) | Windows Arm64 command bundle
[`blackrelay-registry_darwin_amd64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_darwin_amd64.tar.gz) | macOS Intel command bundle
[`blackrelay-registry_darwin_arm64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_darwin_arm64.tar.gz) | macOS Apple Silicon command bundle
[`blackrelay-registry_freebsd_amd64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_freebsd_amd64.tar.gz) | FreeBSD x64 command bundle
[`blackrelay-registry_openbsd_amd64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_openbsd_amd64.tar.gz) | OpenBSD x64 command bundle
[`blackrelay-registry_netbsd_amd64.tar.gz`](https://github.com/blackrelay/registry/releases/latest/download/blackrelay-registry_netbsd_amd64.tar.gz) | NetBSD x64 command bundle

Each command bundle also includes `README.md`, `LICENSE`, `openapi/registry.v1.yaml` and JSON contract files from `contracts/`.

# Integrity Assets

Each release attaches:
- `SHA2-256SUMS`
- `SHA2-256SUMS.sig`
- `SHA2-512SUMS`
- `SHA2-512SUMS.sig`
- `public.key`

The checksum manifests cover every direct binary and command bundle. Black Relay signs checksum manifests rather than every binary file. This keeps the release asset list smaller while still covering each file listed in the manifests.

# Release Signing

Black Relay Registry uses a maintainer-controlled OpenPGP release key for checksum manifest signatures.

The public key attached to a GitHub release is a convenience copy. It is not the trust root by itself. Compare its fingerprint with a maintainer-controlled channel before trusting release signatures.

The release workflow expects:
```text
Secret:   RELEASE_SIGNING_PRIVATE_KEY
Value:    ASCII-armoured private PGP key used only for release manifest signing

Secret:   RELEASE_SIGNING_PASSPHRASE
Value:    passphrase for RELEASE_SIGNING_PRIVATE_KEY

Variable: RELEASE_SIGNING_KEY_FINGERPRINT
Value:    uppercase fingerprint of the release signing key
```

Do not commit private keys, passphrases, deployment SSH keys or local Git identity configuration.

# Release Provenance

The current release flow provides GitHub workflow provenance through the release workflow run and signed checksum manifests. It does not currently publish GitHub artefact attestations, SLSA provenance bundles or SBOM files.

Do not describe Registry release artefacts as reproducible, vulnerability-free or reviewed by a human unless that has been separately verified for the specific release.

# Publishing

Manual publishing requires:
- `version`, for example `v1.0.0` or `v1.0.0-rc.1`;
- `target`, usually `main` or a full commit SHA;
- `publish` set to `true`.

If the version tag does not exist, the workflow creates an annotated tag at the verified target commit before publishing the GitHub release.

Pushing a tag matching `v*` also runs the release workflow. Tags with a prerelease suffix such as `-rc.1` are marked as prereleases.

Run the workflow with `publish` set to `false` for a dry run. The dry run still builds assets and signs manifests because it should exercise the same release path as publication.

# Verification

Verify downloaded files after importing the release public key:
```sh
gpg --import ./public.key
gpg --verify ./SHA2-256SUMS.sig ./SHA2-256SUMS
sha256sum -c ./SHA2-256SUMS --ignore-missing
```

Windows:
```powershell
gpg --import .\public.key
gpg --verify .\SHA2-256SUMS.sig .\SHA2-256SUMS
(Get-FileHash .\br-registry_windows_amd64.exe -Algorithm SHA256).Hash.ToLower()
Select-String -Path .\SHA2-256SUMS -Pattern "br-registry_windows_amd64.exe"
```

The Windows example compares one downloaded file. The Linux example can verify every downloaded file present in the working directory.

# Version Notes
- [v1.0.0](v1.0.0.md): first Registry release notes and verification instructions.
