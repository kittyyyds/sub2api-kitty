# Server-side 2FA login browser

The selected proxy and resolved IANA timezone are frozen before initial login.
Each job owns a Chromium context, cookie jar, PKCE transaction and private proxy
bridge. SOCKS5 destination DNS is resolved by the upstream proxy. HTTP(S) and
authenticated SOCKS5 proxies are supported. Failure never falls back to direct
traffic. OAuth token exchange uses the same proxy and cookies via Playwright's
API request context; it is a standard back-channel exchange, not a second login.

Only ordinary email, password and TOTP fields are automatically submitted.
Challenges, consent and unknown pages pause for the human administrator. The
authenticated SUB2API modal streams screenshots and relays user input to this
same browser. It does not run a CAPTCHA solver, transfer verification cookies
between environments or override browser fingerprints.

Deploy one private worker per site. Set a distinct `KITTY_LOGIN_BROWSER_TOKEN`
(at least 32 random characters). The app additionally receives
`KITTY_LOGIN_BROWSER_URL`. Never publish port 8080. Run as an unprivileged user
with memory/CPU/process limits, no Docker socket, and a writable ephemeral `/tmp`
and home. Default capacity is two sessions per worker. Sessions expire after
20 minutes; abandoned resources are removed no later than 25 minutes.

`npm ci --ignore-scripts` uses the lockfile. The release image is
`mcr.microsoft.com/playwright:v1.56.1-noble` pinned by image digest at deployment.
The worker source and installed pure-JavaScript dependencies can be mounted
read-only in that image; no compilation on the production server is needed.

Run `npm test` with a compatible Chromium installed. Browser integration tests
use synthetic credentials, isolated mock OAuth, authenticated HTTP/SOCKS
proxies and a synthetic verification page. They send no real account login.

New web clients selecting a proxy send `login_environment` and use this worker.
Older API clients omitting that field keep the release-pinned protocol login.
Without a selected proxy, the web UI explicitly keeps the existing login
service; request-time timezone rewrite remains separate in that path.
