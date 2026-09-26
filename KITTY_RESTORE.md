# Kitty Upgrade And Restore Guide

## v0.2.5 candidate — 2026-09-16

Candidate version: `0.2.5-kittyyyds.1`, for `kittyyyds.xyz` only. The upgrade
starts from verified production commit `45231756b9410de0a7b356f6c0930074327c129d`
(`0.2.4-kittyyyds.11`) and merges official `v0.2.5` against official `v0.2.4`.
This section describes the candidate requirements, not completed deployment.

- Retain the current and all historical releases, tags, images and recovery
  sets. The current user's full-history policy supersedes older retention text.
- Build frontend and Linux backend outside the production servers, with
  explicit version/commit/date metadata and embedded frontend. Assemble the
  final image locally from the SHA-256-verified `docker save` archive of this
  site's current immutable image, replacing `/app/sub2api` and the complete
  `/app/resources` directory with an overlay equivalent to
  `deploy/kitty-overlay.Dockerfile`. Bind the base-image ID/archive hash,
  candidate Git commit, binary hash, frontend manifest, resources manifest and
  resulting image ID/archive hash in one release manifest. Verify that manifest
  again before server-side `docker load`; do not compile, install development
  dependencies or assemble build layers on either production host.
- `deploy/offline-overlay.py` writes an OCI archive for Docker's containerd
  image store. Its required build receipt binds the binary SHA-256, source
  commit, version/date, and matching frontend manifests recorded before and
  after compilation. Verify the loaded Linux image and served frontend assets
  before switching traffic.
- A temporary process sharing the live database during traffic handoff must
  set `KITTY_DEPLOYMENT_HANDOFF=true`. This skips only the active channel-monitor
  and scheduled-account-test runners so they cannot duplicate paid probes.
  API requests, manual tests and other workers keep their existing behavior.
  Leave this variable unset on the final production process and restart it to
  restore these two runners; it is read at startup, not dynamically.
- Run the loaded candidate in an isolated Linux smoke environment before
  changing the production image: apply explicit CPU/memory limits, a separate
  port and disposable cloned data/database; keep production customer writes
  outside that instance. Verify Linux startup, embedded pages, migrations and
  required business checks. Earlier named-context build examples are historical
  alternatives; this release uses the reviewed offline overlay tool and manifest.
- Preserve identity v2 and its diagnostics, prompt-cache-key/continuation
  contracts, per-model and per-resolution final image/video prices, recharge
  bonuses, Kitty Apps, candy tests, registration-only Turnstile, group access
  controls and both selected-risk switches resetting off for a new release.
- Do not reintroduce retired Cursor, model variant locks, old Token reward
  handlers, or add the separately discussed automatic account-login feature.
- Keep all 302 already published SQL files unchanged. New official migrations
  `238_opencode_go_platform.sql` and `238_purge_unlimited_user_platform_quotas.sql`
  coexist with Kitty's `238_kitty_candy_tests.sql` by complete filename.
  Preflight and back up the all-null platform-quota rows before the purge;
  retain the additional historical `231_disable_long_context_pricing.sql`
  ledger row without guessing or rewriting its missing source.
- Preserve existing subscription settings while accepting the new public flag,
  navigation and payment-mode behavior. Verify both recharge and subscription
  paths together with the Kitty checkout and bonus displays.
- Regress identity HTTP/WS/compact/tool continuation and peer-close/queueing,
  model prices, payments, Apps, candy results, API-key provider selection/bulk
  editing, registration confirmation, subscription gating and group boundaries.

## Identity diagnostics and protocol repair (2026-09-13)

The candidate based on `0.2.4-kittyyyds.7` adds an admin-only recent-request
diagnostic view, header-only v2 identity projection for legacy compact, and
bounded parent-thread reference resolution. It changes no database schema,
account settings, identity seeds/epochs, prices, business concurrency or request
pacing. Keep the `.7` image and its recovery manifest as the immediate rollback
until the candidate passes production acceptance. Application rollback alone is
sufficient; never restore old balances or usage from a database dump for this fix.

Actual trace events are `identity_v2_outbound_start/result`. Headers received do
not prove response completion. Recent diagnostics and parent reference caches
are bounded and process-local; restarting clears them. Unresolved parents retain
the client's original reference. Native WS terminal summaries do not claim TLS,
per-frame identity or client delivery observations they do not possess.

Regression gate: `TestIdentityV2*`, `TestOpenAIClientProfile*`, compact/service-tier
and continuation tests, administrator route tests, and the two identity Vue
component suites. Verify source/image identity, rollback availability and
memory headroom; compile frontend and Linux binary locally. Server-side image
assembly must stay in the existing bounded worker and stop on low headroom.

## Independent identity v2 cutover

The `0.2.4-kittyyyds.6` candidate replaces the prior custom identity code only on
`kittyyyds.xyz`. Its immediate application rollback is
`kitty-tb/sub2api:0.2.4-kittyyyds.5`, revision
`b489a8aa579fbe2e6db94f246fe1a24899ddaf2c`, image ID
`sha256:079dfcbc24dfe348c853f115e256431d12766ff9d801ead32064659a25b52be6`.

Prepare and checksum the existing recovery set before migration. Migration 226
only prepares enabled v2 seeds and leaves legacy fields intact while `.5` runs.
After `.6` passes health/authentication/payment/catalog checks, enable the
requested account 111 with the Mac declaration and standard TLS. Verify its
request, then archive/remove the legacy identity fields. Other accounts remain
opt-in. A Mac declaration does not emulate a physical device or promise changes
to upstream model quality.

The server-bound release directory is
`/opt/kitty-tb/staging/codex-identity-v2-20260912`. Its `rollback-current.py
--site kittyyyds --check` validates image/configuration and selective identity
recovery; adding `--apply` restores the saved application and identity settings.
It refuses conflicting newer administrator edits. Ordinary rollback does not
restore the PostgreSQL/Redis dump over current usage, balances or credentials.
The checksummed recovery set remains outside Git and is retained with all older
release recovery sets. Production acceptance never authorizes automatic pruning.

## v0.2.4 release requirements

- Preserve Kitty's existing features, registration-only Turnstile, custom payment UI, group visibility boundaries, and selected risk controls. Both selected risk switches must reset off after a version/commit upgrade. Cursor and variant-lock features remain retired; do not import third-party SVIP rules or platform removals.
- Use the official OpenAI weekly total-cost badge only. Keep Luna's selected default input/output prices at 0.40/2.40 USD per million tokens and Astra at 10/50, with existing overrides and multipliers.
- Frontend and the Linux/amd64 Go binary may both be compiled locally. Build the binary with CGO_ENABLED=0, `-tags embed`, and explicit Version/Commit/Date/BuildType metadata after generating Ent/Wire and building the matching frontend. Validate its build settings with `go version -m`.
- When using `KITTY_BACKEND_STAGE=backend-prebuilt`, supply the verified `kitty_backend` named context containing `sub2api`. The release driver must bind its SHA-256 to the clean source commit and frontend manifest and verify both again on the server before assembling the image. Keep the complete source resources directory and default source-build path. The resulting candidate must pass Linux runtime, migration, authorization, billing, and risk-control smoke checks before switching traffic.
- Never compile source or run test compilation concurrently on a production host. Check live memory/disk headroom before any work and use a site-specific worker with hard resource limits for image assembly. Verify the primary site, secondary direct domain, and secondary API domain through the primary site after release.

## Repository baseline

- Upstream tag: `v0.1.173`
- Kitty branch: `kitty/v0.1.173`
- Kitty release version: `0.1.173-kitty.2`
- Immediate rollback image: `0.1.172-kitty.2`
- Previous v0.1.156 baseline customization commit: `a343593`
- Previous Kitty release: `kitty/v0.1.160` at `f78b7b9e`

The repository intentionally excludes `.env`, `config.yaml`, database dumps, API keys, payment secrets, and Cloudflare credentials.

## Production backup retention

`UPGRADE_BACKUP_POLICY.md` is mandatory for every upgrade. As requested on
2026-09-16, this repository retains all historical production release units for
its bound site. A release unit is not merely a branch: it is an annotated production tag,
an immutable image digest, a sanitized deployment manifest, and the checksummed
off-repository PostgreSQL/Redis recovery set.

Create each new production tag only after the candidate passes production and
rollback acceptance. Then make `main` point to that candidate and
`rollback/previous` point to the immediately preceding production tag. Retain
every older tag, Release, image and recovery set. Inspect the retained tag list:

```sh
./deploy/rotate-production-backups.sh <site-id> <primary-domain>
```

The compatibility-named rotation script now only lists locally available
production tags and refuses its former `--apply` mode. It never deletes tags or
off-repository recovery sets. Insufficient space requires separate storage
planning; it does not authorize release-history deletion. Operational/PITR/
compliance backups also remain outside the release script's management scope.

## Upgrade workflow

1. Create a new Kitty branch from the latest verified Kitty branch.
2. Merge the target upstream tag with `--no-ff`, then resolve conflicts by preserving both the upstream feature and the matching Kitty patch.
3. Resolve conflicts first in `payment_order.go`, `payment_fulfillment.go`, `payment_refund.go`, and `usage_billing_repo.go`.
4. Run `go generate ./ent` after resolving schema changes, then run `go generate ./cmd/server` after resolving Wire provider changes.
5. Build the Vue frontend so the embedded `backend/internal/web/dist` output reflects the merged source.
6. Build the production backend exactly like `deploy/Dockerfile`, including `-tags embed`, explicit `VERSION`, `COMMIT`, `DATE`, and `BuildType=release` ldflags. A binary built without the `embed` tag serves APIs but returns `Frontend not found` for the console.
7. Before switching traffic, verify `go version -m` reports `-tags=embed`, start the candidate image without replacing production, and require HTTP 200 from `/`, `/login`, `/register`, `/apps`, `/custom/docs`, `/model-plaza?embedded=1`, and `/admin/settings`.
8. Apply migrations from the same final candidate commit before switching traffic. Do not use an older binary or migration bundle against the release database.
9. Deploy only the embedded Vue application image. Keep the proxy compatibility snippets in `deploy/kitty-site`, but never inject separate UI scripts, styles, or HTML.

## v0.1.173 integration notes

- Apply all upstream migration files `194_channel_monitor_v2.sql` through `206_channel_monitor_v2_privacy_defaults.sql` plus `217_group_video_model_prices.sql`, `218_group_audio_voice_pricing.sql`, `219_group_search_price_per_1k.sql`, and `220_clear_non_grok_video_generation_config.sql`. Keep Kitty migrations with the same numeric prefixes under their original filenames.
- Immediately before deployment, repeat the migration `220` preflight and confirm it will not clear an active third-party Grok/composite group's video pricing. Verify all 17 complete filenames exist in `schema_migrations` after startup.
- Preserve the official deferred Grok video billing state machine together with Kitty's permanent owner binding and original-account history lookup. Pending and failed tasks must not bill; a completed task must bill exactly once whether completion is observed through status or content download.
- Preserve both official JSON image-object normalization and Kitty's OpenAI-compatible multipart forwarding, account model mapping, media aliases, image ratios, 1080p capability checks, signed URL handling, independent media multipliers, Apps checkpoint recovery, and download-only video delivery.
- Explicitly save `grok_cross_client_model_map_enabled=true` after deployment so Codex and Claude Code requests continue using the intended Grok mappings. The upstream release note describes a default-off migration while the tagged code currently treats a missing value as enabled, so production must not depend on the implicit default.
- Keep `channel_monitor_mode=v1` during initial verification. Do not enable V2 backfill in the same change as the application upgrade; production has a large `usage_logs` table and V2 should be enabled as a separate operational change.
- Regenerate Ent and Wire from the merged sources. The final provider graph must include Channel Monitor V2 and all active Kitty providers, especially Apps, recharge bonuses, Grok media history recovery, payment providers, captcha providers, and Codex version synchronization.
- The production candidate image is `0.1.173-kitty.2`. Keep `0.1.172-kitty.2`, `0.1.172-kitty.1`, and the new PostgreSQL/Redis/Compose backups until authentication, all 17 migrations, pricing, text gateways, root Voice/Web Search aliases, cross-client Grok mapping, Grok media billing, historical video downloads, Apps, and the embedded Feishu guide pass.

## v0.1.172 integration notes

- Preserve the `/custom/docs` mapping to `FeishuGuidePanel.vue`, the exact Kitty API Feishu wiki URL, the responsive iframe shell, and its new-window fallback. Keep the Feishu wiki, account, and login origins in both default/example CSP policies and `requiredCSPDirectiveValues` so an older persisted policy is upgraded at runtime.
- Preserve the upstream OAuth pending-session terminal-state guard together with Kitty's registration-only Turnstile proof and retrying public-settings loader. Tencent captcha region values must flow through the shared widget, but Turnstile must not expand to login or password recovery.
- Apply upstream migrations `194_add_usage_log_upstream_response_model.sql` and `195_add_usage_log_upstream_model_mismatch_index_notx.sql` in addition to Kitty's existing `194_application_center.sql` and `195_application_center_optional_planner.sql`. The runner keys by complete filename, and invalid-index recovery must retain both upstream `195` and Kitty `197`.
- Regenerate UsageLog Ent code and the Wire provider graph from the merged tree. The final graph must retain Kitty Apps, recharge bonuses, Grok media history recovery, payment providers, captcha providers, and Codex version synchronization.
- Preserve upstream model-response auditing, monetary `NUMERIC(20,8)` quantization, daily subscription midnight reset, explicit upstream dial timeout, Codex capacity failover, Responses schema cleanup, and Grok `task_id` ownership binding alongside all Kitty gateway and media paths.
- Composite Model Plaza entries must remain distinct by platform plus model name. The production price table must keep platform badges and Kitty's independent image/video rate calculations.
- The production candidate image is `0.1.172-kitty.2`. Keep `0.1.172-kitty.1`, `0.1.171-kitty.2`, and current PostgreSQL/Redis backups until authentication, migrations, pricing, text gateways, Grok media generation, historical video downloads, and the embedded Feishu guide pass.

## v0.1.171 integration notes

- There are no upstream database migrations or Ent schema changes in `v0.1.171`; Kitty migrations through `197_grok_media_history_lookup_indexes_notx.sql` remain the complete migration baseline.
- After merging the upstream provider graph, regenerate `wire_gen.go`; the generated graph must include Kitty Apps, recharge bonuses, the Grok media services, Tencent/Alibaba captcha services, and Codex version synchronization together.
- Preserve the registration-only Turnstile proof flow. The upstream captcha provider abstraction may be used for Tencent/Alibaba action gates, but do not put Cloudflare Turnstile on login or password recovery.
- Preserve the upstream Model Plaza image-price fix together with Kitty's independent image/video rate multipliers and media billing rows.
- The production candidate image is `0.1.171-kitty.2`; keep `0.1.171-kitty.1`, `0.1.170-kitty.3`, and the existing database/Redis backups until post-upgrade health, authentication, pricing, Grok media, and download-only history checks pass.
- Kitty `0.1.171-kitty.2` adds all official Grok image aspect ratios to Kitty Apps. Preserve the shared frontend/backend allowlist, single-image source-ratio behavior, multi-image override, and OpenAI-compatible `size` + `resolution` + validated `aspect_ratio` forwarding together.

## Recharge bonus migration

Migration `backend/migrations/182_recharge_bonus_credits.sql` creates:

- campaign rules and payment-order snapshots;
- per-order bonus grants and an immutable ledger;
- batch-image hold allocations used to preserve expiry semantics.

The migration seeds the initial tiers with `ON CONFLICT DO NOTHING`. Future campaign changes should update `recharge_bonus_rules`; do not edit historical order snapshots or grants.

## Retired Token reward migration history

Migrations `backend/migrations/192_token_reward_claims.sql` and `backend/migrations/193_token_reward_claim_cycle_start.sql` are retained unchanged because deployed databases may already have recorded their checksums. They preserve old claim rows but expose no active application behavior.

Do not restore the retired repository, service, user/admin handlers, routes, Vue pages, API clients, navigation, locale modules, tests, or Wire providers during an upstream merge. The old `token_reward_config` setting may remain in an existing database and is intentionally ignored. Do not add a destructive migration for the ledger unless historical data deletion is approved separately.

## v0.1.161 integration notes

- Upstream adds `181_prompt_audit.sql` and `182_prompt_audit_full_prompt.sql`; the Kitty recharge migration remains `182_recharge_bonus_credits.sql`.
- Migration identity is the complete filename plus checksum, so both `182` files coexist safely. Do not rename the applied recharge migration.
- Upstream `v0.1.161` adds migrations `183_ops_ingress_reject_aggregates.sql` and `184_auth_cache_invalidation_outbox.sql`. Both are additive and do not alter recharge bonus tables.
- Wire is regenerated from the `v0.1.161` provider graph. The new ingress-reject aggregator, auth-cache invalidation worker and subscriber run alongside prompt audit, audit log, billing probe, and Kitty recharge expiry services.
- The OpenAI handler keeps Kitty's configurable first-output failover while accepting the upstream Grok media eligibility prober.
- The frontend uses the upstream SUB2API console skin while keeping the shared favicon updater.
- The prompt audit provider set needs an explicit `PromptAdminService` to `*PromptService` binding when Wire is regenerated.
- Prompt audit is disabled by default and does not alter the existing content moderation configuration until an administrator enables it.
- `@intlify/message-compiler@9.14.5` is a direct development dependency because the upstream locale compilation test imports it directly under pnpm's strict dependency layout.
- Keep the retired Kitty 3D scene, its theme overrides, and its build-time variant setting out of customer builds.
- Keep registration fail-closed when public settings cannot be loaded, and prefer the server-embedded public-settings snapshot before making a forced API retry.
- Build with explicit `VERSION` and `COMMIT` arguments, then verify the runtime version before switching traffic.
- The upstream real-client-IP changes continue to honor the existing trust-forwarded-client-IP setting and do not enable it automatically.
- DujiaoPay uses `keyId`, `apiSecret`, `webhookSecret`, `apiBase`, and `currency`; keep the two secrets outside Git.
- Recreate the DujiaoPay endpoint as `https://kittyapi.xyz/api/v1/payment/webhook/dujiaopay`, subscribe to `order.paid`, and copy the endpoint Webhook Secret into the provider instance after each environment restore.
- PayPal uses `clientId`, `clientSecret`, `webhookId`, `environment`, `currency`, and optional `brandName`; keep the Client Secret outside Git.
- Recreate the PayPal endpoint as `https://kittyapi.xyz/api/v1/payment/webhook/paypal`, subscribe to `CHECKOUT.ORDER.APPROVED`, `PAYMENT.CAPTURE.COMPLETED`, and `PAYMENT.CAPTURE.DENIED`, and keep all three credentials in the same Sandbox or Live app.
- Keep the provider-independent pending-order reconciliation patch when rebasing; it recovers missed callbacks for every payment provider, not only WeChat Pay.
- Keep the EasyPay query-method patch: order status checks must use `GET /api.php?act=order` with bounded transient retries, otherwise gateways such as the production WeChat provider return `No Act!` and paid orders remain pending.

## Removed customization boundaries

- Keep the administrator profit-analysis page, its report and fixed-cost APIs, and its dedicated aggregation code removed during future rebases.
- Preserve the general account billing multiplier, upstream billing probe, usage-cost snapshots, and dashboard cost statistics; those are shared billing capabilities rather than the removed report.
- Keep the customer console on the upstream SUB2API AppLayout and global theme. Do not restore the retired Kitty 3D scene, customer-only theme overrides, Three.js dependency, or scene build variable.

## v0.1.164 integration notes

- Composite groups, composite model routes, group reasoning-effort policies, Grok cooldown handling, OpenAI request normalization, and proxy stream isolation come from upstream unchanged.
- The checkout response includes both Kitty recharge-bonus fields and upstream `alipay_mobile_precreate_deep_link`.
- Mobile Alipay precreate recovery runs alongside provider-independent pending-order recovery; DujiaoPay and PayPal must continue to be actively queried while pending.
- Wire starts and stops both `RechargeBonusExpiryService` and `OllamaCloudUsageService`.
- CC Switch keeps the Codex endpoint on the configured `/v1` base, strips `/v1` only from `usageBaseUrl`, and adds `/v1` to Grok imports when absent.
- Migrations `186_alipay_mobile_precreate_deep_link.sql` and `186_group_auth_cache_image_generation.sql` intentionally coexist because migration identity is the full filename.
- The upstream content-moderation stale-cache test explicitly expires its snapshot. A nanosecond TTL is nondeterministic on Windows clocks and does not test a different production behavior.
- The production image version is `0.1.164-kitty.3`.

## Grok enhancement prompt notes

- Preserve migration `187_group_grok_prompt.sql`, the Ent group fields, auth-cache snapshot mappings, and `internal/service/grok_group_prompt.go` when rebasing.
- Empty `grok_prompt_content` intentionally selects `DefaultGrokEnhancementPrompt`; do not materialize the built-in prompt into every database row.
- Injection belongs at the Grok forwarding boundaries for Responses, Chat Completions, and Messages so retries, raw fallbacks, and protocol bridges share one implementation.
- Keep the marker-based duplicate guard and preserve existing client instructions. Prompt content remains administrator-only in HTTP DTOs.
- After deployment, enable the feature only for the intended Grok group and invalidate API-key auth cache entries for that group through the normal group update path.
- Migration `187` is additive. An older binary ignores the new columns, but the enhancement is inactive while that binary is running.
- Migration `188_enable_grok_enhancement_group.sql` enables the built-in prepend prompt only for the exact active Grok group named `Grok 增强版` and queues auth-cache invalidation for its API keys.

## v0.1.165 integration notes

- ChatGPT Live, Claude Opus 5, usage-log session IDs, announcement preview, request-driven Ollama usage refresh, and normalized registration email aliases come from upstream unchanged.
- Group Ent code must be regenerated after merging because upstream `allow_live` and Kitty's three Grok prompt fields share the generated field-order tables. A textual merge can compile but panic during Ent runtime initialization.
- Upstream migrations `187_add_usage_log_session_id.sql` and `188_allow_live_usage_request_type.sql` intentionally coexist with Kitty migrations `187_group_grok_prompt.sql` and `188_enable_grok_enhancement_group.sql`; migration identity is the complete filename.
- Upstream registration alias normalization strengthens the existing registration-only Turnstile flow and does not move Turnstile onto login or password recovery.
- Preserve Kitty Grok prompt injection after upstream's orphaned `tool_choice` cleanup and pool-mode 5xx scheduling fix.
- Preserve Kitty announcement header actions while accepting upstream's shared rich-text styles and preview dialog.
- The production image version is `0.1.165-kitty.2`.
- The overlay image replaces both `/app/sub2api` and the complete `/app/resources` directory so upstream model-pricing changes are deployed with the binary.
- Production first-output protection uses `90` seconds for ordinary OpenAI Responses requests, `240` seconds for high/xhigh/max reasoning, and at most `2` account switches. These thresholds avoid cutting off legitimate large-context prefill while bounding stalled upstream attempts.
- Welfare group `6` keeps account `11608` as the active route. Accounts `12235`, `12236`, and `12239` remain bound at priority `100` but are unschedulable until their repeated upstream 403 model-access failures are repaired.

## v0.1.166 integration notes

- Panel API rate limiting, per-turn WebSocket billing models, mapped-model usage statistics, partial settings updates, request-ID filtering, Antigravity compatibility, Gemini retry handling, and composite prefix routing come from upstream unchanged.
- Preserve Kitty's registration-only Turnstile bootstrap and bounded fail-closed retries while applying upstream's `affiliate_enabled` setting. The optional affiliate field remains before Turnstile and is hidden when mandatory invitation codes are enabled.
- Preserve Kitty's configurable OpenAI first-output failover while accepting upstream's foreign-reasoning cleanup between passthrough and non-passthrough accounts.
- Payment dashboard totals are grouped by currency. DujiaoPay, PayPal, provider-independent recovery, EasyPay GET reconciliation, recharge bonus snapshots, fulfillment, expiry, and refund behavior remain active.
- Upstream `v0.1.166` adds no database migration files. Existing Kitty migrations and production data remain the migration baseline.
- Caddy disables compression on SSE responses to prevent buffering. Keep the exact `/v1` CC Switch route and separately deployed Kitty site assets when updating the production proxy.
- The production image version is `0.1.166-kitty.3`; replace the complete `/app/resources` directory in the overlay image.
- Do not use a plain cross-compiled `go build` for the production overlay binary. Keep `-tags embed` and probe the origin SPA routes before changing the Compose image tag.
- Preserve the bare-domain customer endpoint contract. The public `api_base_url` setting is `https://kittyapi.xyz`, the key page displays and copies the default route without a trailing `/v1`, and custom endpoints remain exactly as configured because they may use provider-specific paths.
- Preserve the root API compatibility routes in `deploy/kitty-site/root-api-compat.conf` together with the backend's bare `/responses`, `/models`, `/chat/completions`, `/embeddings`, image, and video routes. Verify a missing API key returns JSON `401` rather than the SPA for both bare and `/v1` paths.
- Preserve the endpoint latency panel and its public `/health` CORS headers. Its outer surface, header, and rows intentionally reuse the customer key table's card, table-header, and table-body materials instead of a separate blue panel.

## v0.1.168 integration notes

- Passkey authentication, the model plaza, Kimi K3 support, scoped user/API-key updates, Codex web-search preservation, and the upstream gateway fixes come from `v0.1.168` unchanged.
- Upstream migration `191_passkey_credentials.sql` remains at its published filename. Historical Kitty migrations `192_token_reward_claims.sql` and `193_token_reward_claim_cycle_start.sql` retain their filenames and checksums even though their runtime feature is retired.
- Wire must include Passkey and Model Plaza, but must not reintroduce the retired Token reward providers. Regenerate `wire_gen.go` after every provider-set merge.
- Kitty `0.1.168-kitty.3` adds the unified Grok media studio. Preserve all four generation modes, authenticated video polling/download, task recovery without persisted private keys, non-destructive reference limits, request snapshots, and Grok edit `resolution`/`aspect_ratio` forwarding together.
- Kitty `0.1.168-kitty.4` completes the official Grok media model matrix. Preserve the two selectable image models in both image modes, the two selectable video models in both video modes, 1.5 text-to-video, the image-to-video-only `1080p` rule, fixed official default prices, exact-alias-first account mapping compatibility, compatibility-alias discovery filtering, and mapped-model default-price fallback together.
- Kitty `0.1.168-kitty.5` adds the application center and migration `194_application_center.sql`. Preserve the catalog, dual-key planner/media boundary, strict planner JSON contract, deterministic negative-prompt merge, persisted run and step state, video task checkpoints, retry/cancel behavior, catalog-revision recovery, and browser upload validation together.
- Application-center persistence stores structured inputs, planner output, task IDs, status, and media URLs only. Do not add uploaded media binaries or base64 payloads to PostgreSQL when rebasing or extending a workflow.
- The production image version is `0.1.168-kitty.5`; retain the embedded Kitty frontend, complete resources overlay, and root API compatibility routes.

## v0.1.169 integration notes

- Preserve upstream's closed-list validation for OpenAI response subpaths and Gemini model path segments. The Kitty bare-domain compatibility routes remain separate entry points and must not bypass the upstream path guard.
- Keep upstream's proxy stream circuit fallback behavior and optional `gateway.openai_proxy_stream_circuit.disabled` setting alongside Kitty's configurable first-output timeouts and account-switch limit.
- Upstream adds no database migration or Wire provider changes. Existing Kitty migrations through `195_application_center_optional_planner.sql` remain the complete migration baseline.
- Available-channel composite-group expansion comes from upstream and must coexist with Kitty's platform-filtered supported-model pricing view.
- Production deployment must include `/app/resources` with the embedded binary so the model-pricing fallback catalog is available at runtime.
- Keep `security_opt: no-new-privileges:true` in the production Compose service while retaining the existing Kitty timeout environment variables and proxy configuration.
- Kitty `0.1.169-kitty.2` originally added `free-media-studio` as a client-managed catalog entry. The current Kitty tree supersedes that design: the four-mode workspace requires the persisted Kitty Apps runtime and optional planner migration `195_application_center_optional_planner.sql`. Do not restore `ClientManaged`, a standalone runtime fallback, direct media execution, `kittyImageHistory`, or `kittyVideoTask`. Keep the customer navigation branded `Kitty应用 / Kitty Apps` and attach the existing hot badge to that route.
- Kitty `0.1.169-kitty.3` updates the built-in xAI media rate card to the official resolution-specific prices. Keep Grok group media price overrides empty when multiple Imagine models share a group so the model-specific defaults remain effective; use the image/video multipliers for customer pricing instead.
- Kitty `0.1.169-kitty.4` adds channel-level video billing tiers in USD per second. Preserve the `video` billing mode, resolution-tier editor, duration/count multiplication, and legacy `image`/`per_request` per-call behavior. Production Grok media pricing stores the upstream balance rates per model and resolution in the channel, then applies independent image/video group multipliers of `1.5`.
- Kitty `0.1.169-kitty.5` adds an account-scoped `openai_compatible` Grok media contract for third-party relays. Preserve image `1K/2K` to `1024x1024/2048x2048` normalization, `b64_json` responses, multipart image-edit forwarding with indexed `image[n]` fields and `mask`, the five-value video aspect-ratio whitelist, bounded duration/resolution checks, original 4xx/422 response passthrough, and the official-xAI default path. Application Center uses `json_object` for Grok planners while keeping strict JSON Schema for other planner platforms.
- Kitty `0.1.169-kitty.6` completes the Kitty media migration into Kitty Apps. Preserve the required application runtime, persisted run history and video checkpoints, legacy-route redirects, retired-menu filtering, and release assertions that forbid an external direct media runtime and browser-local media state.
- The final v0.1.169 production image is `0.1.169-kitty.16`; retain it as the immediate rollback image together with the embedded Kitty frontend, complete resources overlay, and root API compatibility routes.

## v0.1.170 integration notes

- Upstream migrations `192_group_profit_control.sql` and `193_group_profit_control_auth_cache_invalidation.sql` intentionally coexist with the retired Kitty ledger migrations `192_token_reward_claims.sql` and `193_token_reward_claim_cycle_start.sql`; migration identity is the complete filename plus checksum.
- Migration `196_group_auth_cache_profit_grok_prompt.sql` supersedes the group invalidation function body from upstream migration `193` without modifying applied history. It invalidates API-key auth snapshots when either the three profit-control fields or the three Grok enhancement-prompt fields change, while retaining all earlier group fields from the upstream function.
- Per-group profit control and the `profit-preview` preflight tool come from upstream. The gate applies only to token-billed scheduling paths, uses a stable request pricing timestamp, rechecks account cost after slot acquisition, and leaves the feature disabled by default.
- Upstream billing-rate probing now covers API-key accounts on OpenAI, Anthropic, Gemini, Grok, and Antigravity. Optional per-account automatic synchronization validates upstream-declared rates before writing them back and prevents concurrent manual multiplier edits while enabled.
- Preserve the upstream Codex OAuth namespace-tool and default-instructions fixes, Grok pool cooldown and billing-ping SSE filtering fixes, Claude Code multi-system classifier fix, and interrupted Anthropic stream-usage accounting fix.
- All active Kitty patches remain merged: Grok enhancement and media routing, Kitty Apps and its persisted four-mode runtime, recharge bonuses, DujiaoPay/PayPal/EasyPay recovery, bare-domain API compatibility, CC Switch import behavior, model pricing and endpoint latency UI, registration-only Turnstile, configurable first-output failover, and the standard SUB2API customer-console skin.
- Kitty `0.1.170-kitty.2` added authenticated content retrieval for completed Kitty Apps history videos. Keep the original-run media-key lookup, `X-Session-Id` header, revocable Blob URL lifecycle, CSP `media-src 'self' blob: https:` enhancement, non-billing video reads, durable owner binding, fail-closed database recovery for expired legacy bindings, migration `197_grok_media_history_lookup_indexes_notx.sql`, `video/mp4` fallback, content disposition normalization, and byte-range forwarding together. Never persist video bytes on local disk.
- Kitty `0.1.170-kitty.3` disables browser playback on all three customer result surfaces and keeps only Blob-backed downloads. Preserve `DisabledVideoPreview.vue`, the native disabled buttons, and the current-result, free-media, and history download links together; do not restore a `<video>` element in any Kitty Apps component.
- The historical v0.1.170 production image was `0.1.170-kitty.3`; retain it as the immediate rollback image for that release together with the embedded Kitty frontend, complete resources overlay, and root API compatibility routes.

## Grok video history authenticated download migration

Migration `backend/migrations/197_grok_media_history_lookup_indexes_notx.sql` adds two partial lookup indexes without changing or backfilling application or usage data:

- `idx_application_run_steps_grok_video_request` indexes video step request IDs and run IDs
- `idx_usage_logs_grok_video_history_owner` indexes the session, user, media key, group, and upstream account evidence on video usage rows

Both statements use `CREATE INDEX CONCURRENTLY`. Run this `_notx.sql` migration through the repository migration runner or execute each statement outside a transaction. Do not wrap it in `BEGIN`, rename it, edit it after application, or insert its `schema_migrations` record manually.

An interrupted concurrent build can leave an index with `pg_index.indisvalid = false`. Rerun the same final candidate: the migration runner checks both exact index names, drops only invalid copies with `DROP INDEX CONCURRENTLY`, recreates them, and records the migration only after both statements succeed. The `usage_logs` index scans the existing usage table and can consume database I/O, so let the migration finish before switching traffic.

Preserve these records when moving or restoring the service:

- PostgreSQL `application_runs`, `application_run_steps`, and `usage_logs`
- The original media API key, its group relationship, and the upstream account credentials
- Redis data when available, including the non-expiring owner bindings created after this release

Redis loss does not authorize guess-based routing. The database fallback restores only Kitty Apps videos whose application step and usage row prove one unique account. Ordinary historical video API calls without an application-run step remain unavailable after their legacy routing binding expires. Quota-exhausted and expired original keys remain usable for this read-only path. A missing or manually inactive original media key, changed group scope, missing usage logs, multiple matching accounts, a removed upstream account, or upstream video expiry still prevents download.

After applying migration `197`, verify that its migration record exists and both indexes are ready and valid:

```sql
SELECT filename
FROM schema_migrations
WHERE filename = '197_grok_media_history_lookup_indexes_notx.sql';

SELECT c.relname, i.indisready, i.indisvalid
FROM pg_class AS c
JOIN pg_namespace AS n ON n.oid = c.relnamespace
JOIN pg_index AS i ON i.indexrelid = c.oid
WHERE n.nspname = 'public'
  AND c.relname IN (
    'idx_application_run_steps_grok_video_request',
    'idx_usage_logs_grok_video_history_owner'
  )
ORDER BY c.relname;
```

The first query must return one row. The second must return both indexes with `indisready` and `indisvalid` set to `true`.

## Verification

```powershell
cd backend
$env:GOCACHE="$PWD\.cache\go-build"
go generate ./ent
go generate ./cmd/server
go test -tags=embed ./cmd/server ./internal/web
go test -tags=unit ./internal/repository
go test -tags=unit ./internal/repository -run 'Test(GrokMediaHistoryLookupIndexesMigration|ApplyMigrationsFS_GrokMediaHistoryLookupIndexesMigration|GrokMediaVideoOwnerAccountQuery|FindGrokMediaVideoOwnerAccountID)' -count=1
go test -tags=unit ./internal/service -run 'Test(ForwardGrokMediaContent|ResolveGrokMediaVideoRequestAccount|GrokMediaVideoRequestBinding)' -count=1
go test -tags=unit ./internal/handler ./internal/server/middleware -run 'Test(GrokMediaRequiresBillingEligibility|APIKeyAuthVideoReadsSkipExhaustedBilling|IsVideoTaskRead|EnhanceCSPPolicy)' -count=1
go test -tags=unit ./internal/payment/provider ./internal/service ./internal/handler -run 'DujiaoPay|PayPal|PaymentProvider|ProviderSnapshot|PaymentTradeNo'
go test ./cmd/server ./internal/repository
go test ./...

cd ..\frontend
corepack pnpm@9.15.9 install --frozen-lockfile
corepack pnpm@9.15.9 run lint:check
corepack pnpm@9.15.9 run typecheck
corepack pnpm@9.15.9 run test:run src/api/__tests__/videoGeneration.spec.ts src/features/app-center/components/__tests__/AppResultPanel.spec.ts src/features/app-center/components/__tests__/AppRunHistory.spec.ts src/features/app-center/components/media/__tests__/FreeMediaStudioPanel.spec.ts src/features/app-center/composables/__tests__/useAppCenter.spec.ts
corepack pnpm@9.15.9 run test:run src/components/__tests__/TurnstileWidget.spec.ts src/views/auth/__tests__/RegisterView.spec.ts src/components/layout/__tests__
corepack pnpm@9.15.9 run test:run src/components/payment
corepack pnpm@9.15.9 run test:run src/views/user/__tests__/PaymentView.spec.ts
corepack pnpm@9.15.9 run test:run
corepack pnpm@9.15.9 run build
```

After deployment, verify the video download-only path:

- Confirm the `/apps` CSP `media-src` directive permits `'self'`, `blob:`, and `https:`
- Open a newly completed Kitty Apps video and confirm no `<video>` element is rendered, the play button is natively disabled, and the download link uses a `blob:` URL
- Open a pre-upgrade completed video whose legacy Redis routing binding expired and confirm the database recovery selects its original account
- Confirm the content request uses the original media key and run session, and still succeeds with exhausted balance or an original key marked `quota_exhausted` or `expired`
- Switch history selections while a download prepares and confirm an older response cannot replace the latest selection
- Compare the application data directory before and after download preparation and confirm no video file was created; expect the video bytes to pass through server bandwidth once per prepared Blob

Also verify one test order for each payment path: first callback, repeated callback, ordinary API billing, bonus expiry, batch-image cancellation after expiry, and a full refund.

## Rollback

Rolling back the binary does not remove Kitty migrations `182`, `192`, `193`, `194`, `195`, `196`, or `197`, or upstream migrations `183`, `184`, `192` through `206`, and `217` through `220`. Migrations `194` through `206` and `217` through `219` are additive structures that the old binary ignores. Migration `220` clears non-Grok video pricing after copying the affected rows into `groups_video_price_backup_220`; restore from that backup table only if the deployment preflight unexpectedly found affected groups. Leave Kitty migration `197` and its two indexes in place during an image rollback because they contain no video bytes and do not change billing data. Keep the previous `0.1.172-kitty.2` image and the deployment-time database/Redis/Compose backups until new and pre-upgrade downloads both pass. Do not delete bonus tables while any granted amount is still included in `users.balance`. Keep the retired Token reward ledger migrations and cycle index as applied history; intentionally rolling back to a pre-retirement binary would re-enable that old functionality.
