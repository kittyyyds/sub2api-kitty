#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "${script_dir}/../.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf "${tmp_root}"' EXIT HUP INT TERM

remote="${tmp_root}/remote.git"
work="${tmp_root}/work"
git init --bare -q "${remote}"
git clone -q "${remote}" "${work}"
git -C "${work}" config user.name test
git -C "${work}" config user.email test@example.invalid

mkdir -p "${work}/deploy"
cp "${repo_root}/deploy/rotate-production-backups.sh" "${work}/deploy/"
cp "${repo_root}/deploy/verify-kitty-site.sh" "${work}/deploy/"
cp "${repo_root}/deploy/kitty-site.env" "${work}/deploy/"
printf 'fixture\n' > "${work}/fixture.txt"
git -C "${work}" add .
git -C "${work}" commit -qm initial
git -C "${work}" branch -M main
git -C "${work}" push -q origin main

site_id=$(sed -n 's/^KITTY_SITE_ID=//p' "${work}/deploy/kitty-site.env")
domain=$(sed -n 's/^KITTY_PRIMARY_DOMAIN=//p' "${work}/deploy/kitty-site.env")
# The verifier accepts only GitHub remotes, so use a fixture verifier here; its
# production behavior is covered by verify-kitty-site-test.sh.
cat > "${work}/deploy/verify-kitty-site.sh" <<'EOF'
#!/bin/sh
set -eu
[ "$1" = "${KITTY_TEST_SITE_ID}" ]
[ "$2" = "${KITTY_TEST_DOMAIN}" ]
EOF

export KITTY_TEST_SITE_ID="${site_id}"
export KITTY_TEST_DOMAIN="${domain}"
for stamp in 20260830T010000Z 20260831T010000Z 20260901T010000Z; do
    tag="prod/${site_id}/${stamp}-v1.0.0-$(git -C "${work}" rev-parse --short HEAD)"
    git -C "${work}" tag -a "${tag}" -m "fixture ${stamp}"
done
git -C "${work}" push -q origin --tags

newest=$(git -C "${work}" tag --list "prod/${site_id}/*" --sort=-refname | sed -n '1p')
previous=$(git -C "${work}" tag --list "prod/${site_id}/*" --sort=-refname | sed -n '2p')
previous_commit=$(git -C "${work}" rev-list -n 1 "${previous}")
git -C "${work}" push -q origin "${previous_commit}:refs/heads/rollback/previous"

dry_run=$(cd "${work}" && sh ./deploy/rotate-production-backups.sh "${site_id}" "${domain}")
printf '%s\n' "${dry_run}" | grep -q '^OK: retain all 3 '
[ "$(printf '%s\n' "${dry_run}" | grep -c '^KEEP ')" = 3 ]
[ "$(git --git-dir="${remote}" tag --list "prod/${site_id}/*" | wc -l | tr -d ' ')" = 3 ]

if (cd "${work}" && sh ./deploy/rotate-production-backups.sh "${site_id}" "${domain}" --apply); then
    echo "ERROR: retired deletion mode unexpectedly succeeded." >&2
    exit 1
fi
[ "$(git --git-dir="${remote}" tag --list "prod/${site_id}/*" | wc -l | tr -d ' ')" = 3 ]
[ "$(git -C "${work}" tag --list "prod/${site_id}/*" | wc -l | tr -d ' ')" = 3 ]
[ "$(git --git-dir="${remote}" rev-parse refs/heads/main)" = "$(git -C "${work}" rev-list -n 1 "${newest}")" ]

echo "OK: all historical production tags remain and deletion mode is refused."
