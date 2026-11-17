<p align="center"><img src="frontend/public/logo.svg" alt="KittyAPI" width="88" /></p>

# KittyAPI

**AI API gateway and operations platform**

KittyAPI is independently maintained for `kittyapi.xyz`. Users can call configured models with a single API key, while operators manage upstream accounts, channels, usage, and procurement costs from one console.

[KittyAPI](https://kittyapi.xyz/) · [Releases](https://github.com/kittyyyds/sub2api-kitty/releases) · [Issues](https://github.com/kittyyyds/sub2api-kitty/issues)

[中文](README.md) · English · [日本語](README_JA.md)

## Features

| Feature | What it does |
| --- | --- |
| Model access | OpenAI-compatible Responses, Chat Completions, and streaming. WebSocket availability for Responses depends on the client and gateway configuration. |
| Accounts and keys | Manage authorized OAuth or API key upstreams with model mapping, groups, account scheduling, and concurrency limits. |
| Usage and billing | Set pricing by model and group, and inspect request records, token usage, and charges. |
| Channel operations | Review channel status, request trends, and charges, and analyze gross margins using configured supplier cost rules. |

## Usage, releases, and feedback

- **API access:** Manage your API key on [KittyAPI](https://kittyapi.xyz/), select an authorized group and model, and follow the setup instructions on the site.
- **Source and upgrades:** Use this repository's [releases](https://github.com/kittyyyds/sub2api-kitty/releases) and the matching source archives. The default branch may differ from the deployed version. Do not use another fork's installers or artifacts to upgrade KittyAPI.
- **Reporting an issue:** Include the version, client, error, and redacted reproduction steps. Do not include keys, OAuth credentials, or customer data.

## Attribution and license

KittyAPI builds on open-source code from [Sub2API](https://github.com/Wei-Shaw/sub2api) and is maintained and released independently. The applicable copyright notices and licenses remain in [LICENSE](LICENSE) and the [third-party component directory](third_party/). Connect only to upstream services you are authorized to use and follow their terms.

Copyright (c) 2026 Wesley Liddick
