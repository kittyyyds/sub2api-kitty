# Supplier Management and Business Operations

Kitty 2.19.0-kitty.7 introduces separate administrator entries at
`/admin/providers` and `/admin/operations`. Existing channel and profit analytics
remain available with their original, explicitly estimated historical formula.

## Suppliers, Wallets and Purchases

A supplier identifies a purchasing relationship, not an inferred URL grouping.
Each supplier can have several wallets with explicit, unique identity labels.
Multiple local accounts may share one wallet. Wallet balance is displayed once;
key quota is not substituted for a wallet balance or summed into one.

Sub2API supports a login password or an access token (optionally a JSON
access_token/refresh_token pair). New API uses a personal or system access token
and, when required, its user ID. Authorizations use the existing fixed-key
encryption service and are never returned in state or key-list responses.
Changing a supplier endpoint with saved authorizations requires clearing them
first. Reads reuse the normal validated upstream transport and explicitly linked
account proxy. Redirects are disabled. A balance read does not invoke a model.

Balances can be refreshed manually or on an explicitly enabled 5-1440 minute
cadence. Automatic work uses the existing managed background process, bounded
claims and the existing leader lock. Failure retains the last successful value
and observation time. Copying a linked account snapshot preserves its original
time rather than making stale observations appear fresh.

Purchases are append-only top-up, refund or opening entries, with cash currency,
conversion to the wallet cost currency, base credits, bonus credits and reference.
Voiding retains the original entry. Operation IDs prevent duplicate submissions;
reusing an ID with different contents fails. Wallet identity and currencies are
fixed after creation. Existing account binding is explicit and version checked.
Reassignments require the administrator to select the move option. These records
do not debit any customer or purchase credits from an upstream.

Supplier key inventory is masked. Creation is an explicit remote mutation;
operation records prevent a repeated or unconfirmed submission from silently
creating a second key. Import resolves the selected complete keys on the server,
uses normal local account validation and group checks, and records a wallet
binding. Local group IDs are explicit; an empty selection skips default groups.
An import lock and account metadata make retries reuse already imported records.

## Frozen Procurement Costs

Migration 324 adds a compact procurement receipt keyed by API key ID and request
ID, independent of usage-log retention and customer money settlement. Existing
request delivery deduplication remains authoritative. Capture starts at the
migration's activation time; prior history is not assigned to today's supplier.

On the first canonical receipt insert, attribution is selected from explicit
binding history and the effective model cost rule at the receipt observation
time. Exact upstream model names override `*`. The snapshot stores price-rule
version, quantities, source names, cost currency, conversion to USD and, where
used, the procurement credit rate. No prompt or response content is stored.

Supported rules are recorded standard valuation times an explicit multiplier,
recorded account credit valuation times a lifetime weighted procurement rate,
per-million-token input/output/cache prices, per image, per video second, per
video and per request. The weighted rate is net recorded payment divided by net
received credits (including bonuses), using purchase entries already known at
observation. It is labeled an estimate, not an actual per-request supplier bill.
Token rules do not claim to price media quantities. Currency conversion is frozen
with the rule; missing conversion prevents USD profit comparisons.

Receipt corrections recompute quantities using the original frozen rule, even
after prices, purchases, names or bindings change. A stale replay with a new log
ID cannot overwrite the canonical receipt. Explicit free price zero is priced;
missing cost remains unknown. Detail deletion does not delete procurement data.
Capture participates in the same transaction as its receipt; rollback rolls it
back. There is no monetary backfill or customer balance mutation.

Administrators can append an invoice-cost reconciliation with a required bill
reference and optional conversion rate. The latest registration is used by the
report, while the original estimate and all registrations remain stored. The
reference is administrator-supplied evidence; this release does not automatically
fetch and match every supplier's billing logs.

## Business Dashboard

The page shows receipt valuation, cost coverage, estimated/reconciled/unknown
costs, comparable gross profit, trends, breakdowns and paginated receipts. It
supports supplier, wallet, account, model and group filters within 31 days.
Comparable profit only includes receipts with both known valuation and USD cost.
It is not net operating profit or proof of subscription cash revenue.

Site-wide payment receipts and customer credit balances retain their own labeled
scope. Purchase cash is grouped by actual payment currency. Customer refunded
credits are not presented as actual gateway cash refunds. Monitoring costs keep
their retained-source coverage limits. Source amounts in different currencies
are not silently added together. No balance difference is treated as a
per-request supplier bill.

## Publication and Compatibility

The schema is additive, with short migration lock acquisition and no historical
scan. Managed instances still require a separate migrate-only process. A
rollback binary based on .kitty.6 must explicitly recognize migration 324's exact
reviewed checksum before it can restart against the new ledger. Do not remove
migration entries or restore an old database over live customer accounting.
