#!/bin/sh
set -eu

# Compatibility entry point: historical releases are retained in full.
# This script has no fetch, push, tag deletion, or recovery-file deletion path.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "${script_dir}/.." && pwd)
config_file="${script_dir}/kitty-site.env"

usage() {
    echo "Usage: $0 <site-id> <primary-domain>" >&2
    exit 64
}

if [ "$#" -eq 3 ] && [ "$3" = "--apply" ]; then
    echo "ERROR: automatic release deletion is disabled; all historical releases must be retained." >&2
    exit 65
fi
[ "$#" -eq 2 ] || usage

sh "${script_dir}/verify-kitty-site.sh" "$1" "$2"
. "${config_file}"
prefix="prod/${KITTY_SITE_ID}/"
tags=$(git -C "${repo_root}" tag --list "${prefix}*" --sort=-refname)
tag_count=$(printf '%s\n' "${tags}" | sed '/^$/d' | wc -l | tr -d ' ')

printf '%s\n' "${tags}" | while IFS= read -r tag; do
    if [ -n "${tag}" ]; then echo "KEEP ${tag}"; fi
done
echo "OK: retain all ${tag_count} locally known production tag(s) for ${KITTY_SITE_ID}; no deletion performed."
