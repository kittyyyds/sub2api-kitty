# Kitty Site Binding

This repository is the private source repository for the primary Kitty site only.

| Item | Value |
| --- | --- |
| Site ID | `kittyapi` |
| Primary domain | `kittyapi.xyz` |
| GitHub repository | `kittyyyds/sub2api-kitty` |
| Compose project | `sub2api` |
| Compose service | `sub2api` |
| Deployment path | `/opt/1panel/docker/compose/sub2api` |
| Application port | `8080` |

Before any build or deployment, run:

```sh
./deploy/verify-kitty-site.sh kittyapi kittyapi.xyz
```

The command must succeed before publishing this repository. It verifies the
expected site ID, domain, and Git remote. Never deploy this repository to
`kittyyyds.xyz`.

Runtime secrets, customer records, databases, uploaded files, and server
configuration are not part of this repository and must remain isolated from
the second site.

All upgrades and backups must follow `UPGRADE_BACKUP_POLICY.md`. This site
independently retains every historical production release unit. No release,
tag, image, source archive, or recovery set is automatically rotated or deleted.
