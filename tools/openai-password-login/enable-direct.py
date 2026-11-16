"""Allow an explicitly selected direct exit; keep configured proxies fail-closed."""
from pathlib import Path

path = Path('/opt/vm2api/codex-login-runtime/src/tls-transport.mjs')
assert (path.parent.parent/'PINNED_COMMIT').read_text().strip() == '8548397e89bf80e508eda64a87e0d556d43abc84'
source = path.read_text()
old = 'this.enabled && (!loginInput.proxyUrl || proxy !== new URL(loginInput.proxyUrl).toString())'
new = 'this.enabled && (loginInput.proxyUrl ? proxy !== new URL(loginInput.proxyUrl).toString() : Boolean(proxy))'
assert source.count(old) == 1, 'Pinned runtime layout differs; refusing to patch'
path.write_text(source.replace(old, new))
