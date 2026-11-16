from pathlib import Path
path=Path('/opt/vm2api/codex-login-runtime/src/protocol-login.mjs')
assert (path.parent.parent/'PINNED_COMMIT').read_text().strip()=='8548397e89bf80e508eda64a87e0d556d43abc84'
source=path.read_text()
old='export { run };'
new='export { run, ProtocolClient, createSentinelHeaders, verifyPassword, completeTotpMfaIfNeeded, finishCodexOauth, runCodexOauth, buildSub2apiOauthExport };'
assert source.count(old)==1
path.write_text(source.replace(old,new))
