# Kitty v2.17.7 merge and financial cutover

Date: 2026-10-28. Source: codex. Production baseline: b15c96eb508e78a06db1bb9fccf98a29d387e104 (2.17.0-kitty.5).

## Preserved customization
- Schema-v2 model billing remains explicit group_id + model, enabled, always/context and threshold_tokens. No implicit global model surcharge, no automatic Luna 10x default. Existing 500K rules remain strict greater-than thresholds. Customer cost is not applied to upstream account cost a second time.
- IQ admission (initial concurrency 1 and scheduling disabled, six correct tests before concurrency 2), IQ isolation, concurrency upgrades, proxy history/assignment and platform scope, original login engine choices.
- Kitty theme/navigation, media model price synchronization and classified public price table, unified canvas/wallet, direct upstream video download and paid media retention, recharge bonus accounting.
- No retired observer role or channel context-price editor is restored. Historical unscoped billing rules remain visibly retired.

## Upstream improvements incorporated
Durable usage journal/recovery and exact-money quote audit; per-job batch-image monetary ownership; independent platform quota projection; relogin diagnosis/budget; traffic metadata accounting; channel operations; quality isolation ownership and content moderation categories. Optional functions are not automatically enabled and administrator prices/configuration are not changed.

## Merge adaptations
- Wire retains Kitty services and uses the durable billing worker provider.
- New billing recovery obeys managed active/standby leadership, records in-flight cycles, drains naturally, and cannot restart after Stop. Request-owned usage pool remains alive during background drain.
- Quota-flusher startup and periodic work obey the same leader boundary; final shutdown flush remains synchronous.
- Repositories retain recharge-bonus consume/reserve/capture/release logic inside the new atomic and owned-money operations.
- Test fixtures use the actual Kitty group-scoped schema. Non-admin/retired role authentication remains denied.

## Deployment and rollback boundary
User approved a short maintenance cutover after tests, not a mixed-version rolling upgrade. No new code may serve alongside the old financial writer. Stop new admission and await existing streams/jobs/accounting; never force-kill an active stream. Stop the old process and verify legacy quota dirty backlog before migrating. Preserve all 413 previous migration checksums and apply the 11 explicit new files. Never restore an old database over current balances.

The pre-traffic rollback build contains the old Kitty business code plus identical new migrations ONLY. It may be used only before new financial states exist, with usage_billing_events, usage_billing_quotes, batch_image_money_holds and usage_platform_quota_totals all empty. It is NOT an approved rollback after public traffic resumes; after that, forward repair or a separately tested fully compatible financial build is required.

Rehearsal uses a network-none temporary PostgreSQL and private synthetic fixtures, production schema plus migration ledger only; no customer rows, credentials or billable model calls. Logs and manifests are retained under the dedicated deployment directory.
