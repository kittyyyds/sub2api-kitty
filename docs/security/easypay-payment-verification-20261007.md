# EasyPay payment verification hardening

Date: 2026-10-07. Release: 2.9.9-kitty.2.

Signed payment-request fields could previously be reinterpreted as notification fields while preserving the same ambiguous signing input. Notification signatures alone must not authorize account credit.

This patch discards user-supplied return-URL queries, validates the documented notification shape without merging or collapsing parameters, and checks actual paid-order evidence through the configured payment platform before payment confirmation or fulfillment. The query must provide the matching merchant, merchant order number, platform transaction number, and exact payable amount; errors remain unverified. Query redirects and contradictory JSON evidence are rejected. Signatures and merchant credentials are omitted from new error messages.

The same verification covers callback, reconciliation and old paid-order fulfillment retries. A private request-scoped proof avoids a second query during a successful flow. Legacy provider ambiguity fails closed, while explicit non-EasyPay payment bindings retain their existing fulfillment behavior. Legacy missing transaction numbers may be filled only from verified platform results.

PostgreSQL advisory transaction locks serialize transaction-number ownership checks and paid-state transitions across cooperating app instances. They are application-enforced protection, not a database unique constraint. Separate sites retain separate databases; returned merchant order numbers must match exactly, and new order identifiers carry UUID entropy in 31 characters. The identifiers remain compatible with official WeChat's 32-character limit. Existing cross-site order collisions must be checked before deployment.

Existing balance redemption, permanent recharge bonuses, affiliate rules and subscription fulfillment remain in place. Late create-payment responses can no longer overwrite details after an order leaves pending status. This patch does not reverse historical credits or change account balances administratively.

Validation includes provider and webhook parsing, amount/identity mismatch and query outage rejection, once-only credit, legacy retries, unrelated payment providers, payment recovery, bonus/affiliate regression, and an opt-in disposable PostgreSQL concurrency/rollback test. Tests use synthetic data and local/mock providers, not production callbacks. The PostgreSQL test requires EASYPAY_TEST_POSTGRES_DSN pointing at a disposable loopback database named easypay_security_test.

Builds preserve the previously released site-specific frontend and migration bytes. Deploy by warming an isolated managed instance, switching new requests, handing over background leadership, and draining old requests. Do not re-enable vulnerable payment handling as an unexamined rollback. Existing risky balances and historical orders need separate evidence-based accounting decisions.
