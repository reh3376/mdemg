#!/usr/bin/env bash
# MDEMG Installer — downloads and installs the mdemg binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
#
# Options (via environment variables):
#   INSTALL_DIR   — target directory (default: ~/.local/bin)
#   VERSION       — specific version to install (default: latest)
#   CHANNEL       — release channel: "stable" (default) or "edge" (latest main build)

set -euo pipefail

REPO="reh3376/mdemg"
BINARY_NAME="mdemg"
DEFAULT_INSTALL_DIR="${HOME}/.local/bin"

# --- Helpers ---

info()  { printf '\033[0;34m[info]\033[0m  %s\n' "$1"; }
warn()  { printf '\033[0;33m[warn]\033[0m  %s\n' "$1"; }
error() { printf '\033[0;31m[error]\033[0m %s\n' "$1" >&2; exit 1; }

need_cmd() {
    if ! command -v "$1" > /dev/null 2>&1; then
        error "Required command not found: $1"
    fi
}

# --- Platform Detection ---

detect_os() {
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        darwin) echo "darwin" ;;
        linux)  echo "linux" ;;
        *)      error "Unsupported OS: $os. Supported: darwin, linux" ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        arm64|aarch64) echo "arm64" ;;
        x86_64|amd64)  echo "amd64" ;;
        *)             error "Unsupported architecture: $arch. Supported: arm64, amd64" ;;
    esac
}

# --- Version Detection ---

detect_latest_version() {
    need_cmd curl
    local url="https://api.github.com/repos/${REPO}/releases/latest"
    local version
    version="$(curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')"
    if [ -z "$version" ]; then
        error "Could not determine latest version from GitHub API"
    fi
    echo "$version"
}

# --- Download & Verify ---

download_and_verify() {
    local version="$1"
    local os="$2"
    local arch="$3"
    local install_dir="$4"
    local channel="$5"

    local checksums_name="checksums.txt"
    local tmpdir
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    local asset_name base_url
    if [ "$channel" = "edge" ]; then
        # Edge: bare binary named mdemg-{os}-{arch}
        asset_name="${BINARY_NAME}-${os}-${arch}"
        base_url="https://github.com/${REPO}/releases/download/edge"
    else
        # Stable: tar.gz archive from goreleaser
        local ver_no_v="${version#v}"
        asset_name="${BINARY_NAME}_${ver_no_v}_${os}_${arch}.tar.gz"
        base_url="https://github.com/${REPO}/releases/download/${version}"
    fi

    info "Downloading ${asset_name}..."
    curl -fsSL -o "${tmpdir}/${asset_name}" "${base_url}/${asset_name}" \
        || error "Failed to download ${asset_name}. Check that ${version} has a release for ${os}/${arch}."

    info "Downloading checksums..."
    curl -fsSL -o "${tmpdir}/${checksums_name}" "${base_url}/${checksums_name}" \
        || error "Failed to download checksums file"

    info "Verifying SHA256 checksum..."
    local expected_hash
    expected_hash="$(grep "${asset_name}" "${tmpdir}/${checksums_name}" | awk '{print $1}')"
    if [ -z "$expected_hash" ]; then
        error "Checksum for ${asset_name} not found in checksums.txt"
    fi

    local actual_hash
    if command -v sha256sum > /dev/null 2>&1; then
        actual_hash="$(sha256sum "${tmpdir}/${asset_name}" | awk '{print $1}')"
    elif command -v shasum > /dev/null 2>&1; then
        actual_hash="$(shasum -a 256 "${tmpdir}/${asset_name}" | awk '{print $1}')"
    else
        error "Neither sha256sum nor shasum found — cannot verify checksum"
    fi

    if [ "$expected_hash" != "$actual_hash" ]; then
        error "Checksum mismatch!\n  Expected: ${expected_hash}\n  Actual:   ${actual_hash}"
    fi
    info "Checksum verified."

    mkdir -p "$install_dir"

    if [ "$channel" = "edge" ]; then
        # Edge: bare binary — copy directly
        if [ -w "$install_dir" ]; then
            mv "${tmpdir}/${asset_name}" "${install_dir}/${BINARY_NAME}"
        else
            info "Elevated permissions required to install to ${install_dir}"
            sudo mv "${tmpdir}/${asset_name}" "${install_dir}/${BINARY_NAME}"
        fi
    else
        # Stable: extract tar.gz archive
        info "Extracting to ${install_dir}..."
        tar -xzf "${tmpdir}/${asset_name}" -C "$tmpdir"

        if [ ! -f "${tmpdir}/${BINARY_NAME}" ]; then
            error "Binary '${BINARY_NAME}' not found in archive"
        fi

        if [ -w "$install_dir" ]; then
            mv "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
        else
            info "Elevated permissions required to install to ${install_dir}"
            sudo mv "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
        fi
    fi

    chmod +x "${install_dir}/${BINARY_NAME}"
    info "Installed ${BINARY_NAME} ${version} to ${install_dir}/${BINARY_NAME}"
}

# --- Path Check ---

check_path() {
    local install_dir="$1"
    case ":${PATH}:" in
        *":${install_dir}:"*) ;;
        *)
            warn "${install_dir} is not in your PATH."
            echo ""
            echo "  Add it to your shell profile:"
            echo ""
            echo "    export PATH=\"${install_dir}:\$PATH\""
            echo ""
            ;;
    esac
}

# --- Main ---

main() {
    need_cmd curl

    local os arch version install_dir channel
    os="$(detect_os)"
    arch="$(detect_arch)"
    channel="${CHANNEL:-stable}"
    install_dir="${INSTALL_DIR:-${DEFAULT_INSTALL_DIR}}"

    if [ "$channel" = "edge" ]; then
        version="edge"
    else
        need_cmd tar
        version="${VERSION:-$(detect_latest_version)}"
    fi

    info "Platform: ${os}/${arch}"
    info "Channel:  ${channel}"
    info "Version:  ${version}"
    info "Target:   ${install_dir}"
    echo ""

    download_and_verify "$version" "$os" "$arch" "$install_dir" "$channel"
    check_path "$install_dir"

    echo ""
    info "Done! Run 'mdemg version' to verify."
}

main "$@"
