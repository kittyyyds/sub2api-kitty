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
- Retain all historical production release units for this site, including
  source archives, tags, GitHub Releases, images, and paired recovery sets.
  Never rotate or delete older releases automatically.
- A production release unit includes an annotated `prod/<site-id>/...` tag, an
  immutable image digest, a sanitized configuration manifest, and references
  plus checksums for the matching off-repository PostgreSQL/Redis recovery set.
- Never commit runtime secrets, `.env`, database/Redis dumps, customer data,
  uploads, TLS private keys, tokens, or credentials.
- After production acceptance, point `main` to the new release and
  `rollback/previous` to the immediately previous release without deleting any
  historical tag, release, image, or recovery set. Cleanup requires a new,
  explicit user instruction identifying the exact items.
- Database downgrades and destructive restores always require a separate,
  explicit approval. Code/image rollback does not imply reverse migrations.

The authoritative procedure and naming scheme are in
`UPGRADE_BACKUP_POLICY.md`.
