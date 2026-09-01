#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "${script_dir}/.." && pwd)
config_file="${script_dir}/kitty-site.env"
keep_count=2

usage() {
    echo "Usage: $0 <site-id> <primary-domain> [--apply]" >&2
    exit 64
}

[ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
expected_site_id=$1
expected_domain=$2
mode=${3:-}
[ -z "${mode}" ] || [ "${mode}" = "--apply" ] || usage

"${script_dir}/verify-kitty-site.sh" "${expected_site_id}" "${expected_domain}"
. "${config_file}"

prefix="prod/${KITTY_SITE_ID}/"
git -C "${repo_root}" fetch --quiet origin \
    "+refs/heads/main:refs/remotes/origin/main" \
    "+refs/heads/rollback/previous:refs/remotes/origin/rollback/previous" \
    "+refs/tags/${prefix}*:refs/tags/${prefix}*"

tags=$(git -C "${repo_root}" tag --list "${prefix}*" --sort=-refname)
tag_count=$(printf '%s\n' "${tags}" | sed '/^$/d' | wc -l | tr -d ' ')

if [ "${tag_count}" -le "${keep_count}" ]; then
    echo "OK: ${tag_count} production tag(s); nothing to rotate."
    exit 0
fi

newest=$(printf '%s\n' "${tags}" | sed -n '1p')
previous=$(printf '%s\n' "${tags}" | sed -n '2p')

for tag in "${newest}" "${previous}"; do
    [ "$(git -C "${repo_root}" cat-file -t "${tag}")" = "tag" ] || {
        echo "ERROR: ${tag} is not an annotated tag." >&2
        exit 65
    }
done

main_commit=$(git -C "${repo_root}" rev-parse refs/remotes/origin/main)
newest_commit=$(git -C "${repo_root}" rev-list -n 1 "${newest}")
rollback_commit=$(git -C "${repo_root}" rev-parse refs/remotes/origin/rollback/previous)
previous_commit=$(git -C "${repo_root}" rev-list -n 1 "${previous}")

[ "${main_commit}" = "${newest_commit}" ] || {
    echo "ERROR: origin/main does not match newest production tag ${newest}." >&2
    exit 65
}
[ "${rollback_commit}" = "${previous_commit}" ] || {
    echo "ERROR: rollback/previous does not match ${previous}." >&2
    exit 65
}

old_tags=$(printf '%s\n' "${tags}" | sed -n '3,$p')
echo "KEEP ${newest}"
echo "KEEP ${previous}"
printf '%s\n' "${old_tags}" | while IFS= read -r tag; do
    [ -n "${tag}" ] && echo "DELETE ${tag}"
done

if [ "${mode}" != "--apply" ]; then
    echo "DRY RUN: rerun with --apply only after the paired recovery manifests are verified."
    exit 0
fi

printf '%s\n' "${old_tags}" | while IFS= read -r tag; do
    [ -n "${tag}" ] || continue
    case "${tag}" in
        "${prefix}"*) git -C "${repo_root}" push --quiet origin --delete "refs/tags/${tag}" ;;
        *)
            echo "ERROR: refusing unexpected tag ${tag}." >&2
            exit 65
            ;;
    esac
done

echo "OK: retained the two newest successful production tags for ${KITTY_SITE_ID}."
