"""Overlay diagnostics on the exact already prepared runtime; no login execution."""
from pathlib import Path

path = Path('/opt/vm2api/codex-login-runtime/src/tls-transport.mjs')
assert (path.parent.parent/'PINNED_COMMIT').read_text().strip() == '8548397e89bf80e508eda64a87e0d556d43abc84'
source = path.read_text()
old = 'import { loginInput } from "./vm2api-hooks.mjs";'
assert source.count(old) == 1
source = source.replace(old, 'import { loginInput, reportLoginResponse } from "./vm2api-hooks.mjs";')
old = '      const riskControl = isRiskControlResult(result);'
assert source.count(old) == 1
source = source.replace(old, '''      reportLoginResponse({status: Number(result.status), headers: result.headers,
        body: result.body ? Buffer.from(result.body, "base64").toString("utf8") : "", url});
      const riskControl = isRiskControlResult(result);''')
path.write_text(source)
