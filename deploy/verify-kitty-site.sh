#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "${script_dir}/.." && pwd)
config_file="${script_dir}/kitty-site.env"

if [ ! -r "${config_file}" ]; then
    echo "ERROR: Missing site identity file: ${config_file}" >&2
    exit 66
fi

# This tracked file contains identifiers only; deployment secrets never belong here.
. "${config_file}"

: "${KITTY_SITE_ID:?KITTY_SITE_ID is required}"
: "${KITTY_PRIMARY_DOMAIN:?KITTY_PRIMARY_DOMAIN is required}"
: "${KITTY_GITHUB_REPOSITORY:?KITTY_GITHUB_REPOSITORY is required}"

expected_site_id="${1:-}"
expected_domain="${2:-}"

if [ -z "${expected_site_id}" ]; then
    echo "Usage: $0 <site-id> [primary-domain]" >&2
    exit 64
fi

if [ "${expected_site_id}" != "${KITTY_SITE_ID}" ]; then
    echo "ERROR: Site mismatch. Expected '${expected_site_id}', repository is '${KITTY_SITE_ID}'." >&2
    exit 65
fi

if [ -n "${expected_domain}" ] && [ "${expected_domain}" != "${KITTY_PRIMARY_DOMAIN}" ]; then
    echo "ERROR: Domain mismatch. Expected '${expected_domain}', repository is '${KITTY_PRIMARY_DOMAIN}'." >&2
    exit 65
fi

if ! origin_url=$(git -C "${repo_root}" remote get-url origin 2>/dev/null); then
    echo "ERROR: This directory has no Git origin remote." >&2
    exit 69
fi

case "${origin_url}" in
    "https://github.com/${KITTY_GITHUB_REPOSITORY}"|"https://github.com/${KITTY_GITHUB_REPOSITORY}.git"|"git@github.com:${KITTY_GITHUB_REPOSITORY}"|"git@github.com:${KITTY_GITHUB_REPOSITORY}.git"|"ssh://git@github.com/${KITTY_GITHUB_REPOSITORY}"|"ssh://git@github.com/${KITTY_GITHUB_REPOSITORY}.git")
        ;;
    *)
        echo "ERROR: Git origin does not match ${KITTY_GITHUB_REPOSITORY}." >&2
        exit 65
        ;;
esac

echo "OK: ${KITTY_SITE_ID} | ${KITTY_PRIMARY_DOMAIN} | ${KITTY_GITHUB_REPOSITORY}"
