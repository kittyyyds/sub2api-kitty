# Kitty Site Binding

This repository is the private source repository for the secondary Kitty site only.

| Item | Value |
| --- | --- |
| Site ID | `kittyyyds` |
| Primary domain | `kittyyyds.xyz` |
| GitHub repository | `kittyyyds/sub2api-kittyyyds` |
| Compose project | `kittyyyds` |
| Compose service | `app` |
| Deployment path | `/opt/kitty-tb` |
| Application port | `18080` |

Before any build or deployment, run:

```sh
./deploy/verify-kitty-site.sh kittyyyds kittyyyds.xyz
```

The command must succeed before publishing this repository. It verifies the
expected site ID, domain, and Git remote. Never deploy this repository to
`kittyapi.xyz`.

Runtime secrets, customer records, databases, uploaded files, and server
configuration are not part of this repository and must remain isolated from
the primary site.

All upgrades and backups must follow `UPGRADE_BACKUP_POLICY.md`. This site
independently retains all historical source releases, tags, images, and recovery
sets. Upgrades never trigger automatic pruning or overwrite an older release.
