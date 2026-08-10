#!/usr/bin/env bash
set -euo pipefail

detect_os() {
    local raw_os
    raw_os="$(uname -s)"
    case "${raw_os}" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)
            echo "Unsupported OS: ${raw_os}" >&2
            exit 1
            ;;
    esac
}

detect_arch() {
    local raw_arch
    raw_arch="$(uname -m)"
    case "${raw_arch}" in
        x86_64)          echo "amd64" ;;
        aarch64|arm64)   echo "arm64" ;;
        *)
            echo "Unsupported architecture: ${raw_arch}" >&2
            exit 1
            ;;
    esac
}

resolve_version() {
    local plugin_dir="${1}"

    # HELM_PLUGIN_VERSION is set by Helm from plugin.yaml during hook execution.
    # Use it when it's a real release version (not the dev placeholder).
    local helm_ver="${HELM_PLUGIN_VERSION:-}"
    if [[ -n "${helm_ver}" && "${helm_ver}" != "0.0.0" && "${helm_ver}" != "dev" ]]; then
        echo "${helm_ver}"
        return
    fi

    # Fallback: read the tag from git. Reliable when Helm checks out a specific
    # tag via `helm plugin install URL --version vX.Y.Z`, but HELM_PLUGIN_VERSION
    # is still the placeholder (tag commit predates the release workflow update).
    local git_version
    git_version=$(git -C "${plugin_dir}" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
    if [[ -n "${git_version}" ]]; then
        echo "${git_version}"
        return
    fi

    echo "Error: could not determine plugin version. Install from a tagged release or ensure plugin.yaml is up to date." >&2
    exit 1
}

verify_checksum() {
    local archive_path="${1}"
    local archive_name="${2}"
    local checksums_path="${3}"

    local expected_sum
    expected_sum="$(grep "${archive_name}" "${checksums_path}" | awk '{print $1}')"
    if [[ -z "${expected_sum}" ]]; then
        echo "Checksum entry for ${archive_name} not found in checksums.txt" >&2
        exit 1
    fi

    local actual_sum
    if command -v sha256sum &>/dev/null; then
        actual_sum="$(sha256sum "${archive_path}" | awk '{print $1}')"
    elif command -v shasum &>/dev/null; then
        actual_sum="$(shasum -a 256 "${archive_path}" | awk '{print $1}')"
    else
        echo "Neither sha256sum nor shasum is available; cannot verify checksum" >&2
        exit 1
    fi

    if [[ "${actual_sum}" != "${expected_sum}" ]]; then
        echo "Checksum mismatch for ${archive_name}" >&2
        echo "  expected: ${expected_sum}" >&2
        echo "  actual:   ${actual_sum}" >&2
        exit 1
    fi
}

main() {
    local os arch version archive_name archive_url checksums_url
    local tmp_dir bin_dir bin_name dest_path

    os="$(detect_os)"
    arch="$(detect_arch)"
    version="$(resolve_version "${HELM_PLUGIN_DIR}")"

    archive_name="vigie_${os}_${arch}.tar.gz"
    archive_url="https://github.com/fregateops/vigie/releases/download/v${version}/${archive_name}"
    checksums_url="https://github.com/fregateops/vigie/releases/download/v${version}/checksums.txt"

    echo "Installing vigie ${version} (${os}/${arch})..."

    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "${tmp_dir}"' EXIT

    echo "Downloading ${archive_url}..."
    curl --fail --location --progress-bar \
        --output "${tmp_dir}/${archive_name}" \
        "${archive_url}"

    echo "Downloading checksums..."
    curl --fail --location --silent \
        --output "${tmp_dir}/checksums.txt" \
        "${checksums_url}"

    echo "Verifying checksum..."
    verify_checksum "${tmp_dir}/${archive_name}" "${archive_name}" "${tmp_dir}/checksums.txt"
    echo "Checksum OK"

    echo "Extracting binary..."
    tar -xzf "${tmp_dir}/${archive_name}" -C "${tmp_dir}" vigie

    bin_dir="${HELM_PLUGIN_DIR}/bin"
    mkdir -p "${bin_dir}"

    bin_name="vigie_${os}_${arch}"
    dest_path="${bin_dir}/${bin_name}"

    cp "${tmp_dir}/vigie" "${dest_path}"
    chmod +x "${dest_path}"

    echo "vigie ${version} installed to ${dest_path}"
}

main "$@"
