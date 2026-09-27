#!/usr/bin/env sh
# Render the Scoop manifest for a published release.
#
# Like the Homebrew formula, the manifest installs the release archives, so
# each Windows architecture pins its own zip and digest, read from the
# SHA256SUMS the release already publishes.
set -eu

REPOSITORY_URL="https://github.com/KoukeNeko/taiga-cli"

usage() {
    printf '%s\n' "usage: $0 <version-tag> <sha256sums-file>" >&2
    exit 2
}

[ "$#" -eq 2 ] || usage
tag=$1
checksums=$2

case "$tag" in
    v*) version=${tag#v} ;;
    *)
        printf '%s\n' "version tag $tag must start with v" >&2
        exit 2
        ;;
esac

case "$version" in
    *-*)
        printf '%s\n' "refusing to render a manifest for pre-release $tag" >&2
        exit 2
        ;;
esac

[ -f "$checksums" ] || {
    printf '%s\n' "no such checksum file: $checksums" >&2
    exit 2
}

digest_for() {
    archive="taiga_${version}_$1.zip"
    value=$(awk -v name="$archive" '$2 == name { print $1 }' "$checksums")
    [ -n "$value" ] || {
        printf '%s\n' "no digest for $archive in $checksums" >&2
        exit 1
    }
    printf '%s' "$value"
}

windows_amd64=$(digest_for windows_amd64)
windows_arm64=$(digest_for windows_arm64)

# checkver and autoupdate stay in the manifest so Scoop's own tooling can
# still bump it by hand if a release ever misses this step.
cat <<MANIFEST
{
    "version": "$version",
    "description": "Independent command-line client for Taiga",
    "homepage": "$REPOSITORY_URL",
    "license": "MIT",
    "notes": "Taiga is a trademark of its respective owners. This is an independent client, not affiliated with or endorsed by the Taiga project.",
    "architecture": {
        "64bit": {
            "url": "$REPOSITORY_URL/releases/download/$tag/taiga_${version}_windows_amd64.zip",
            "hash": "$windows_amd64",
            "extract_dir": "taiga_${version}_windows_amd64"
        },
        "arm64": {
            "url": "$REPOSITORY_URL/releases/download/$tag/taiga_${version}_windows_arm64.zip",
            "hash": "$windows_arm64",
            "extract_dir": "taiga_${version}_windows_arm64"
        }
    },
    "bin": "taiga.exe",
    "checkver": {
        "github": "$REPOSITORY_URL"
    },
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "$REPOSITORY_URL/releases/download/v\$version/taiga_\$version_windows_amd64.zip",
                "extract_dir": "taiga_\$version_windows_amd64"
            },
            "arm64": {
                "url": "$REPOSITORY_URL/releases/download/v\$version/taiga_\$version_windows_arm64.zip",
                "extract_dir": "taiga_\$version_windows_arm64"
            }
        },
        "hash": {
            "url": "$REPOSITORY_URL/releases/download/v\$version/SHA256SUMS"
        }
    }
}
MANIFEST
