# Kitty Custom Patches

This image tracks upstream Sub2API `v0.1.173` on branch `kitty/v0.1.173`. The Kitty release version is `0.1.173-kitty.2` and carries the production-specific changes below.

## v0.1.173 upstream integration

- Allow embedded builds to reach the upstream root aliases for Voice and Web Search (`/tts`, `/stt`, `/custom-voices`, `/realtime`, `/web_search`) instead of serving the SPA shell.

- Grok SSO/refresh-token authorization, shared OAuth sessions, media routing, Voice APIs, custom voices, web search, free-tier scheduling gates, team/model cooldowns, streaming failover, and observed-model quota handling are merged without removing Kitty's API-key and third-party OpenAI-compatible Grok channels.
- The official asynchronous video billing fix now stores create-time pricing, charges only a completed task, and claims the charge once. Kitty's permanent owner binding, original-account history recovery, authenticated download-only delivery, signed upstream URL handling, model mapping, aspect ratios, and independent image/video multipliers remain intact.
- Official image-object normalization and the three-source edit limit coexist with Kitty's multipart forwarding and endpoint capability rules: `grok-imagine-video-1.5` stays explicit and supports 1080p generation, while reference-to-video, edit, and extension remain on the legacy video model.
- Channel Monitor V2 and its passive gateway-traffic aggregation are included alongside V1. Production remains on V1 for the first release verification so the existing usage-log table is not backfilled during the application upgrade.
- Email-domain registration quota is merged behind its default-off switch while Kitty's registration-only Turnstile proof, fail-closed public-settings loader, retry behavior, and localized registration copy remain active.
- Upstream migrations `194` through `206` and `217` through `220` coexist with Kitty migrations that share numeric prefixes because migration identity uses the complete filename. Migration `220` was preflighted against production and currently finds no non-Grok/composite group with video pricing to clear.
- The Grok cross-client model-mapping setting must be saved explicitly as enabled in production to preserve Kitty's Codex and Claude Code routing behavior. Do not rely on the missing-setting default because the release note and tagged runtime differ.
- Gemini native image accounting, detached upstream image context, bounded Grok import probing, response-model hot-path optimization, and OpenAI OAuth routing-hint updates are included.

## v0.1.172-kitty.2 embedded getting-started guide

- The authenticated `新手必看` entry at `/custom/docs` embeds the Kitty API Feishu guide in the customer console and keeps a visible `新窗口打开` fallback.
- The guide panel owns only iframe loading and responsive presentation; the existing route, sidebar item, and header shortcut remain stable.
- Default, example, and runtime-enhanced CSP policies allow the exact Feishu wiki, account, and login origins so persisted production policies do not block redirects.
- Keep `FeishuGuidePanel.vue`, its registry mapping, and all three CSP origins together during future upstream rebases. The former native `DocumentationPanel.vue` remains in the tree as reference content but is no longer mapped to `docs`.

## v0.1.172 upstream integration

- The pending OAuth completion flow includes the upstream account-takeover fix: non-terminal sessions cannot bind an attacker-controlled identity to an existing account. Kitty's registration-only Turnstile proof and bounded public-settings retries remain intact, while Tencent captcha region handling follows the corrected upstream contract.
- Usage logs retain the upstream response model and mismatch flag so administrators can detect upstream model substitution. Upstream migrations `194_add_usage_log_upstream_response_model.sql` and `195_add_usage_log_upstream_model_mismatch_index_notx.sql` coexist with Kitty migrations that use the same numeric prefixes but different full filenames.
- Composite Model Plaza groups preserve platform plus model identity and show cross-platform badges. Kitty's text, image, video, and per-request price ledgers continue to apply the effective group or independent media multiplier.
- Codex pre-output capacity failover, the `codex-tui` outbound identity, Responses tool-schema normalization, Anthropic invalid-block filtering, explicit upstream dial timeouts, daily subscription reset repair, monetary precision quantization, system-log backoff, Gemini 3.6 Flash, and Grok 405/image-cooldown fixes are included.
- Grok video ownership accepts both `id` and `task_id` receipts without removing Kitty Apps checkpoint recovery, original-account history lookup, authenticated download-only content delivery, or media billing.
- Default and example CSP policies include the corrected Tencent regional endpoints and worker support while retaining `media-src 'self' blob: https:` for Kitty media downloads.

## v0.1.171-kitty.2 Grok image ratios

- Kitty Apps exposes all xAI-supported Grok image aspect ratios for text-to-image and multi-image editing.
- A single-image edit follows the source image ratio and omits `aspect_ratio`; multi-image editing keeps the selected ratio override.
- The application catalog and Grok media compatibility layer share the same allowlist. OpenAI-compatible relay requests retain their square `size` compatibility field while also forwarding `resolution` and a validated `aspect_ratio` where xAI allows it.
- The three affected application definitions advance their revision and version so cached forms cannot keep the former square-only controls.

## v0.1.171 upstream integration

- Tencent Turing and Alibaba Cloud Captcha 2.0 are available as additional, mutually exclusive captcha providers. Kitty keeps Cloudflare Turnstile registration-only behavior and its email-bound 15-minute registration proof; Tencent and Alibaba action verification remains available for OAuth and passkey entry points without expanding Turnstile to login or password recovery.
- Codex OAuth outbound identity, originator, User-Agent, and version are normalized through the upstream release source. The six-hour official Codex version sync and the configurable identity-normalization compatibility switch are retained alongside Kitty's CC Switch and first-output failover behavior.
- Composite-group reasoning policies, OpenAI reset-credit details, Stripe refund idempotency, explicit force confirmation for insufficient-balance refunds, serialized subscription renewals, cancellation-aware scheduler snapshots, and usage-log retention on billing failure come from upstream unchanged.
- Model Plaza image display pricing now follows the effective billing order (group image-tier price, channel tier price, then channel default). Kitty's independent image/video multipliers and expanded media pricing table remain in place.
- Default and runtime CSP policies include the Tencent/Alibaba captcha origins while retaining `media-src 'self' blob: https:` for Kitty's download-only media results.

## v0.1.170 upstream integration

- Upstream per-group profit control and the `profit-preview` preflight tool are merged. Profit thresholds filter token-billed account candidates while preserving normal ranking, sticky-session, retry, and circuit-breaker behavior among eligible accounts.
- Upstream billing-rate probing now covers every supported API-key platform. Optional per-account rate synchronization can write validated upstream-declared rates back to the account while preventing manual edits from racing the managed value.
- The Codex OAuth namespace-tool and missing-instructions fixes, Grok pool cooldown and billing-ping filtering fixes, and Claude Code multi-system classifier plus interrupted Anthropic stream-usage fixes are included.
- Migration `196_group_auth_cache_profit_grok_prompt.sql` keeps profit-control and Grok enhancement fields in one durable API-key auth-cache invalidation function.
- All active Kitty customizations documented below remain part of the merged tree, including Grok enhancement and media support, Kitty Apps, recharge bonuses, payment integrations, bare-domain API compatibility, model pricing, endpoint latency checks, and the standard SUB2API customer-console skin.

## Grok group enhancement prompt

- Grok groups can enable a built-in, client-neutral enhancement prompt or store a custom prompt up to 32 KiB.
- `/v1/responses`, `/v1/chat/completions`, and `/v1/messages` merge the enhancement without replacing client, project, or user instructions.
- Administrators choose prepend or append order; a stable marker prevents duplicate injection across retries and protocol bridges.
- Prompt configuration is included in API-key auth snapshots so forwarding remains on the zero-query hot path, while prompt content is exposed only by administrator group APIs.
- Empty prompt content selects the compact built-in prompt, avoiding the latency and token cost of sending the original multi-file Grok bundle on every request.

## Payment overpay tolerance

- Underpayment is rejected.
- Currencies with at most two minor-unit digits accept overpayment up to `0.05`.
- Currencies with more minor-unit digits accept up to five minor units.
- Accepted overpayments emit a `PAYMENT_OVERPAY_ACCEPTED` audit event.

## OpenAI first-output failover

`gateway.openai_first_output_max_switches` makes the upstream hard-coded one-switch limit configurable from `1` to `10`. Production uses `2` so two primary accounts can fail before routing to the standby account.

## Registration-only Turnstile

Cloudflare Turnstile is shown only on the normal registration page. The verified registration flow exchanges the one-time Turnstile token for a signed, email-bound proof that remains valid for 15 minutes while the user enters or resends an email verification code. Login, password recovery, and pending OAuth account completion do not invoke Turnstile.

- Registration consumes the public settings already injected into `index.html`, avoiding a duplicate cold-start API request behind Cloudflare.
- If security settings are unavailable, registration remains closed while bounded retries run; it never falls back to an unprotected form.
- The Turnstile loader handles new, in-flight, and previously inserted scripts, retries failed loads, and uses explicit rendering without a replaceable global callback.

## Default SUB2API customer console skin

- The authenticated customer console uses the upstream SUB2API AppLayout and global theme.
- The retired Kitty 3D scene, customer-only theme overrides, Three.js dependency, and scene build variable stay out of production builds.
- Kitty business features remain available through the standard SUB2API layout and navigation.

## Canonical image model presets

- The native image studio exposes separate 1K, 2K, and 4K presets.
- Every preset displays and sends the canonical `gpt-image-2` model instead of resolution-suffixed model aliases.
- Resolution remains an independent request-size setting, so selecting a preset never changes the upstream model identifier.

## Unified Grok media studio

- The customer media workbench supports text-to-image, image-to-image, text-to-video, and image-to-video in one native interface while retaining `gpt-image-2`.
- Both image modes expose the official `grok-imagine-image` and `grok-imagine-image-quality` models. Both video modes expose `grok-imagine-video` and `grok-imagine-video-1.5`; the retired synthetic `grok-imagine-edit` model is never advertised.
- `grok-imagine-video-1.5` remains available for text-to-video. `1080p` is accepted only for image-to-video with 1.5, while `reference_images` and the video edit/extension endpoints remain limited to `grok-imagine-video`.
- Default Grok media billing follows xAI's fixed model prices: `$0.02` per standard image, `$0.05` per quality image, `$0.05` per classic-video second, and `$0.08` per 1.5-video second. Group-level channel prices can still override these defaults.
- Historical aliases and explicit account model mappings remain routable, but compatibility-only media aliases stay out of model discovery. Exact client aliases take priority; when an alias has no channel price, billing falls back to its mapped official model before upstream capability validation.
- Video generation uses the application-center gateway for authenticated submit, status, and content requests. Persisted run checkpoints own task recovery; the browser keeps no separate video-task record.
- Completed Kitty Apps history videos are fetched with the run's original media API key and persisted session ID. If that key is outside the initial key list, the client retrieves the exact key by ID. Active, quota-exhausted, and expired keys remain valid for this read-only path; a missing or manually inactive key is never replaced with another key.
- The client downloads the protected content with `Authorization` and `X-Session-Id`, normalizes a missing or generic Blob media type to `video/mp4`, and creates a revocable Blob URL for the authenticated download link. Selecting another history item aborts stale detail/content requests and revokes their URLs; changing the result or leaving the page also revokes the previous URL.
- Never bind `/v1/videos/:id/content` directly to a `<video>` element because media elements cannot attach the required bearer header. Completed-video surfaces intentionally render a disabled play control and an authenticated download action only. The default Content Security Policy (CSP) still includes `media-src 'self' blob: https:` for rollback compatibility, and runtime CSP enhancement adds the same values to persisted custom policies.
- Video request ownership uses a non-expiring Redis owner binding plus the existing short-lived routing binding. A live legacy routing binding is promoted on first read. If both bindings are missing, database recovery requires one application video step and one unique upstream account proven by the same request ID, user, media key, group, run session, and video usage log; missing or ambiguous evidence fails closed. Keep migration `197_grok_media_history_lookup_indexes_notx.sql` and the migration-runner recovery for interrupted concurrent index builds with this fallback.
- Authenticated video status and content reads skip balance, key-quota, and subscription billing enforcement because generation was already charged. Credential validation, user and group access, IP restrictions, the original media-key scope, and request ownership still apply.
- Video content proxy responses forward request ranges and preserve `Content-Length`, `Content-Range`, and `Accept-Ranges`. They normalize missing or generic media types to `video/mp4` and preserve the authenticated content path. The proxy streams bytes without writing a server-side video file; Kitty Apps materializes the response as a browser Blob for download, so each download preparation still transfers the video through the server and temporarily uses browser memory.
- Grok image edits preserve `resolution` and `aspect_ratio` when multipart uploads are normalized to the upstream JSON contract.
- Image and video reference pools remain separate; switching between OpenAI and Grok limits does not discard selected files.
- Keep the mode switch, output panel, application-center gateway, checkpoint recovery, request snapshots, and Grok multipart forwarding changes together when rebasing.

## Application center workflows

- The customer application center provides three optional-planner workflows plus the fully managed `free-media-studio` workflow.
- Free media keeps the flexible form and output experience, but key loading, platform selection, gateway execution, billing, task checkpoints, cancellation, recovery, and history all use the application-center runtime. The panel requires this runtime and has no standalone or legacy image/video client mode.
- `free-media-studio` enters the same persisted `StartRun -> next_action -> Checkpoint/CompleteStep -> history` lifecycle as every other application. Its planner is disabled by default, so an original prompt creates the media step directly; migration `195_application_center_optional_planner.sql` makes the planner key optional.
- Image modes accept GPT Image or Grok media keys and derive the visible model contract from the selected key. Video modes filter to Grok keys before submission, and the backend independently rejects non-Grok video execution.
- Planning and media execution use separate user-owned API keys. Planner output is validated as structured JSON with `final_prompt`, `negative_prompt`, and `rationale`; the negative prompt is merged deterministically into the media request.
- Media keys must belong to the current user and remain active in an enabled image-generation group. Catalog model, platform, mode, aspect-ratio, and resolution constraints are enforced in both the browser and backend.
- Runs, steps, inputs, planner JSON, upstream task IDs, media URLs, cancellation, failures, and retries are persisted. Uploaded media binaries and base64 payloads are never stored in PostgreSQL.
- Video task IDs are checkpointed before polling, survive browser or service interruptions, cannot be replaced by a different task ID, and remain attached when completion or failure updates omit the ID.
- The free-media stop button stops local polling while preserving a checkpointed video run. The shared run-status control remains the explicit destructive cancellation path, and non-checkpointed stopped runs are cancelled instead of being left permanently running.
- Free media displays the shared run status and run history. Browser-local image history and browser-local video task recovery have been removed; do not restore either path.
- Current results, free-media results, and run history use the shared disabled-video preview. Keep all three free of `<video>` elements while preserving their Blob-backed download actions.
- Native Vue router redirects preserve `/image-generator` and `/custom/image-generator` compatibility with `/apps?app=free-media-studio`. Do not restore a separately deployed media runtime, API-key loader, or browser-local media history.
- Active runs retain their catalog revision snapshot, while compatible newer catalogs may resume unfinished work after a deployment.
- Preserve migrations `194_application_center.sql` and `195_application_center_optional_planner.sql`, the repository/service/handler provider graph, all four catalog entries, optional planner semantics, the single free-media runtime, checkpoint-safe stopping, upload validation, shared status/history views, the `Kitty应用 / Kitty Apps` navigation branding and its hot badge together when rebasing. Keep the native router compatibility redirect and release-contract assertions with them. Do not restore `ClientManaged`, optional runtime props, direct media execution, or browser-local media history.

## CC Switch import compatibility

- Provider imports keep the Codex endpoint on the configured `/v1` API base.
- The deep link supplies a separate usage base URL with the terminal `/v1` removed, preventing the bundled usage script from requesting `/v1/v1/usage`.
- The reverse proxy handles the exact `/v1` path directly so CC Switch reachability checks do not follow the automatic `/v1/` redirect.

## DujiaoPay stablecoin checkout

- `dujiaopay` is a first-class payment provider for hosted USDT/USDC checkout.
- API requests use DujiaoPay's six-line HMAC-SHA256 canonical signature and an idempotency key derived from the Sub2API order number.
- `order.paid` webhooks are verified against the raw request body with the project Webhook Secret.
- Webhooks without `merchant_order_id` fall back to an exact, provider-scoped `payment_trade_no` lookup.
- Provider key ID and currency snapshots prevent a callback from crediting an order created under another merchant configuration.
- API Secret and Webhook Secret remain masked in admin responses and are never stored in this repository.

## PayPal hosted checkout

- `paypal` is a first-class payment provider backed by PayPal Orders v2.
- Sandbox and Live use PayPal's fixed official API origins; administrators cannot redirect credentials to a custom API base.
- Client ID, Client Secret, Webhook ID, environment, settlement currency, and an optional checkout brand name are configured per provider instance.
- Return-page and order-query recovery Capture approved orders, while verified Webhooks process approved and completed Capture events idempotently.
- Completed orders persist the PayPal Capture ID so full and partial refunds use the correct upstream transaction.
- PayPal verifies Webhook signatures through its official verification endpoint before Sub2API credits an order.
- Client Secret stays masked in admin responses and is never stored in this repository.

## Provider-independent payment recovery

- The periodic payment-order sweep actively queries every configured provider for recent pending orders instead of limiting recovery to WeChat Pay.
- The authenticated checkout UI performs the same bounded recovery query for Alipay, WeChat Pay, DujiaoPay, PayPal, and custom payment methods.
- Fulfillment remains idempotent, so a delayed webhook and an active query cannot credit the same order twice.

## EasyPay query recovery

- EasyPay order reconciliation uses the standard `GET /api.php?act=order` contract; some gateways reject form-encoded `POST` queries with `No Act!`.
- Transient transport failures, HTTP 429 responses, and upstream 5xx responses are retried three times with bounded backoff.
- Transport errors are sanitized before logging so the query URL cannot expose the merchant key.

## Recharge bonus credits

- Active tiers are stored in `recharge_bonus_rules`; the initial campaign is `30/1/1d`, `50/2/3d`, `100/5/10d`, `300/15/15d`, `500/30/30d`, and `1000/50/90d`.
- A rule snapshot is attached to the payment order at creation, so later campaign edits do not change an existing unpaid order.
- Payment callbacks grant the bonus idempotently and invalidate the cached balance.
- API billing consumes the earliest-expiring bonus first. A scheduled service removes unused expired credit from `users.balance`.
- Batch image holds retain their grant allocation. Unused held credit is restored only when it is still valid, preventing cancellation from converting promotional credit into permanent balance.
- Successful refunds revoke the unused bonus from that order, prorated for partial refunds.
- The checkout API and native Vue checkout read campaign tiers from the backend instead of hard-coded copy.

## Retired token usage rewards

- The Token reward runtime is retired. Do not restore its Vue pages, API clients, routes, handlers, service, repository, provider graph, or admin configuration during future upstream merges.
- Migrations `192_token_reward_claims.sql` and `193_token_reward_claim_cycle_start.sql` remain only as immutable migration history for databases that already applied them. Current application code does not read or write the old claim ledger or the `token_reward_config` setting.
