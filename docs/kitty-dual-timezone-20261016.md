# Dual Request Timezone Policies

Date: 2026-10-16. Source: codex.

Selective integration of upstream v2.11.1 request-timezone forwarding. No
billing, session-export, database-migration or login-worker changes are included.

- Explicit account timezone policies retain first priority, including OFF,
  proxy detection, independent search sync and the existing 2FA login flow.
- Unconfigured OpenAI accounts inherit the global switch. It stays OFF when
  missing or empty, unlike upstream's default-ON behavior.
- The global source defaults to automatic egress detection; manual remains
  selectable. Automatic detection runs after native transport exit selection,
  including regional routing and runtime fallback. HTTP headers and JSON body
  are rewritten on a cloned request. WS handshakes and frames use the selected
  connection exit; pool compatibility distinguishes global modes.
- Automatic detection caches success for 60 seconds and failures for 15 seconds,
  with a 3-second probe budget. Failure leaves the request unchanged. It does
  not change dates, ordinary conversation, system timezone or language.
- Native forwarding only: separately managed plugin and BPS paths are not a
  promise of coverage. Rotating proxies cannot guarantee probe IP equals the
  IP of every subsequent business connection.
- This source note is not proof of production deployment or real-account login.

Upstream source: https://github.com/ranxi2001/sub2api/tree/v2.11.1
The existing LGPL attribution for request-locale parsing is retained.
