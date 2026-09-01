# Kitty repository operating rules

This is a production-bound private repository. Before changing, backing up,
building, or deploying it, read `KITTY_SITE.md`, `UPGRADE_BACKUP_POLICY.md`, and
`KITTY_RESTORE.md` completely.

Mandatory rules:

- Run `./deploy/verify-kitty-site.sh <site-id> <primary-domain>` before any
  build, backup tag, push, or deployment. Never deploy one Kitty repository to
  the other Kitty site.
- Treat `main` as the source of the current verified production release. Use a
  short-lived `upgrade/<site-id>/<YYYYMMDD>-<slug>` branch for upgrade work.
- Retain exactly the two newest successful production release units for this
  site. A failed or unverified candidate is not a production backup and must
  not trigger rotation.
- A production release unit includes an annotated `prod/<site-id>/...` tag, an
  immutable image digest, a sanitized configuration manifest, and references
  plus checksums for the matching off-repository PostgreSQL/Redis recovery set.
- Never commit runtime secrets, `.env`, database/Redis dumps, customer data,
  uploads, TLS private keys, tokens, or credentials.
- Delete the oldest production tag and its paired recovery set only after the
  new release passed production acceptance, `main` points to the new release,
  and `rollback/previous` points to the immediately previous release.
- Database downgrades and destructive restores always require a separate,
  explicit approval. Code/image rollback does not imply reverse migrations.

The authoritative procedure and naming scheme are in
`UPGRADE_BACKUP_POLICY.md`.
