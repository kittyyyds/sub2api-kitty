# Kitty Production Compatibility Rules

The Kitty homepage, authentication screens, console, and checkout are native Vue
components embedded in the application image. Production reverse proxies must
serve the application response unchanged. Do not inject scripts, stylesheets, or
alternate HTML into `index.html`.

- `frontend/public/logo.svg`, `favicon.svg`, `favicon.ico`,
  `apple-touch-icon.svg`, and `og-image.svg` are embedded brand fallbacks. The
  admin-managed `site_logo` setting remains the runtime source for application
  surfaces and browser icons.
- Montserrat is bundled through `@fontsource-variable/montserrat`; homepage and
  authentication components must not depend on Google Fonts at runtime.
- `ccswitch-v1-location.conf` is the exact `/v1` reverse-proxy route used by CC
  Switch reachability checks. Place it before an existing `location ^~ /v1/`
  block when OpenResty is used.
- `root-api-compat.conf` provides aliases for API clients configured with the
  bare site URL instead of `/v1`. Its `/usage` rule preserves the Vue console
  route for browser navigations.

After deploying a new image, verify that `/home`, `/login`, `/register`, and
`/purchase` reference only hashed `/assets/` bundles. Requests for retired
`/kitty-head.js`, `/kitty-auth.js`, `/kitty-auth.css`, and
`/kitty-radar-data.js` must return `404` and must never appear in page markup.
