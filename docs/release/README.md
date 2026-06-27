# Release

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/blackrelay/registry?label=Release&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/releases/latest "Latest Release")
[![Release Workflow](https://img.shields.io/github/actions/workflow/status/blackrelay/registry/release.yml?label=Release%20Workflow&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/actions/workflows/release.yml "Release Workflow")
[![SHA-256](https://img.shields.io/badge/SHA--256-signed-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](https://github.com/blackrelay/registry/releases/latest/download/SHA2-256SUMS "Signed SHA-256 Manifest")
[![Public Key](https://img.shields.io/badge/Public%20Key-release-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](https://github.com/blackrelay/registry/releases/latest/download/public.key "Release Signing Public Key")
[![Licence](https://img.shields.io/badge/Licence-Apache--2.0-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](../../LICENSE "Apache-2.0 Licence")

</div>

Black Relay Registry releases are built by `.github/workflows/release.yml`. The workflow verifies the repository, builds command archives, writes checksum manifests, signs those manifests with the release PGP key and optionally publishes a GitHub release.

## Signing Material

Keep private signing material outside the repository. Store it in operator-managed private storage with restrictive file permissions and back it up through the operator's normal secret-management process.

The release workflow needs one dedicated PGP key for release artefact signing. Export the private key as ASCII armour and store it as a GitHub secret. Store the passphrase separately as a second secret. Store the public fingerprint as a GitHub variable so the workflow can verify that the expected key was imported.

Do not commit private keys, passphrases, deployment SSH keys or local Git identity configuration.

## GitHub Secrets And Variables

Set these in the repository or release environment before publishing a release:
```text
Secret:   RELEASE_SIGNING_PRIVATE_KEY
Value:    ASCII-armoured private PGP key used only for release manifest signing

Secret:   RELEASE_SIGNING_PASSPHRASE
Value:    passphrase for RELEASE_SIGNING_PRIVATE_KEY

Variable: RELEASE_SIGNING_KEY_FINGERPRINT
Value:    uppercase fingerprint of the release signing key
```

The workflow exports the matching public key as `public.key` for consumers.

## Release Assets

The release asset builder produces command archives for:
- Linux amd64 and arm64
- Windows amd64 and arm64
- macOS amd64 and arm64
- FreeBSD amd64
- OpenBSD amd64
- NetBSD amd64

Each platform archive includes `README.md`, the Apache-2.0 licence file, `openapi/registry.v1.yaml` and JSON contract files from `contracts/`.

The workflow writes:
```text
SHA2-256SUMS
SHA2-256SUMS.sig
SHA2-512SUMS
SHA2-512SUMS.sig
public.key
```

`public.key` is the exported release signing public key. The `.sig` files are detached ASCII-armoured PGP signatures over the checksum manifests.

Archive names use this pattern:

Platform | Archive name
:--- | :---
Linux amd64 | `blackrelay-registry_<version>_linux_amd64.tar.gz`
Linux arm64 | `blackrelay-registry_<version>_linux_arm64.tar.gz`
Windows amd64 | `blackrelay-registry_<version>_windows_amd64.zip`
Windows arm64 | `blackrelay-registry_<version>_windows_arm64.zip`
macOS amd64 | `blackrelay-registry_<version>_darwin_amd64.tar.gz`
macOS arm64 | `blackrelay-registry_<version>_darwin_arm64.tar.gz`
FreeBSD amd64 | `blackrelay-registry_<version>_freebsd_amd64.tar.gz`
OpenBSD amd64 | `blackrelay-registry_<version>_openbsd_amd64.tar.gz`
NetBSD amd64 | `blackrelay-registry_<version>_netbsd_amd64.tar.gz`

## Manual Dry Run

Run the release workflow manually with `publish` set to `false` to verify the full release build and upload the generated assets as a workflow artefact. The signing secrets are still required because the dry run signs the manifests in the same way as a published release.

## Publishing

Manual publishing requires:
- `version`, for example `v0.1.0` or `v0.1.0-rc.1`
- `target`, usually `main` or a full commit SHA
- `publish` set to `true`

If the version tag does not exist, the workflow creates an annotated tag at the verified target commit before publishing the GitHub release.

Pushing a tag matching `v*` also runs the release workflow. Tags with a prerelease suffix such as `-rc.1` are marked as prereleases.

## Version Notes
- [v1.0.0](v1.0.0.md): first Registry release notes and verification instructions.

## Verification

Consumers can verify a downloaded archive with:
```sh
gpg --import public.key
gpg --verify SHA2-256SUMS.sig SHA2-256SUMS
sha256sum -c SHA2-256SUMS
```

The `sha256sum -c` command verifies every archive listed in the manifest. To check one archive manually, compare its digest against the matching line in the verified manifest.
