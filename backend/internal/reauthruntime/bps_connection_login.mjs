import crypto from 'node:crypto';

const redirect = 'https://bps.openai.com/basispoints/extension/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3/auth/callback';
export function validateAuthorization(value) {
  const u = new URL(value);
  for (const key of u.searchParams.keys()) if (u.searchParams.getAll(key).length !== 1) throw new Error('BPS_AUTH_URL_INVALID');
  if (u.origin !== 'https://auth.openai.com' || u.pathname !== '/api/accounts/authorize' || u.username || u.password || u.hash ||
      u.searchParams.get('client_id') !== 'app_fnr0pYvVwwFDocDumLG3H2Bp' || u.searchParams.get('redirect_uri') !== redirect ||
      u.searchParams.get('response_type') !== 'code' || u.searchParams.get('code_challenge_method') !== 'S256' ||
      !u.searchParams.get('code_challenge') || !u.searchParams.get('state') || !u.searchParams.get('nonce')) throw new Error('BPS_AUTH_URL_INVALID');
  return u;
}
export function validateCallback(value, state) {
  const u = new URL(value), expected = new URL(redirect);
  for (const key of ['code', 'state', 'client_id', 'error']) if (u.searchParams.getAll(key).length > 1) throw new Error('BPS_CALLBACK_INVALID');
  if (u.origin !== expected.origin || u.pathname !== expected.pathname || u.username || u.password || u.hash ||
      u.searchParams.get('state') !== state || !u.searchParams.get('code') || u.searchParams.has('error') ||
      (u.searchParams.has('client_id') && u.searchParams.get('client_id') !== 'app_fnr0pYvVwwFDocDumLG3H2Bp')) throw new Error('BPS_CALLBACK_INVALID');
  return u.toString();
}
export async function loginBPSConnection(client, options, helpers) {
  const {authBase, email, password, totpSecret, rl} = options;
  const auth = validateAuthorization(process.env.KITTY_BPS_AUTH_URL || 'about:blank');
  if (authBase !== auth.origin) throw new Error('BPS_AUTH_URL_INVALID');
  console.log('[bps] Opening authorization.');
  let page = await client.follow(auth.toString());
  const deviceId = client.jar.value('oai-did', authBase) || crypto.randomUUID();
  if (!helpers.isPasswordLoginPage(page.finalUrl)) {
    if (!helpers.isAuthLoginPage(page.finalUrl)) throw new Error('BPS_INTERACTION_REQUIRED');
    const headers = await helpers.createSentinelHeaders(client, {authBase, deviceId, flow: 'authorize_continue'});
    const {data} = await helpers.authJsonStep(client, authBase, 'POST', '/api/accounts/authorize/continue',
      {username: {kind: 'email', value: email}}, {referer: page.finalUrl, headers});
    const next = helpers.getContinueUrl(data);
    if (!next || !helpers.isPasswordLoginPage(next)) throw new Error('BPS_INTERACTION_REQUIRED');
    page = await client.follow(next, {referer: page.finalUrl});
  }
  console.log('[3/5] Password login page reached.');
  let payload = await helpers.verifyPassword(client, {authBase, rl, deviceId, password, referer: page.finalUrl});
  payload = await helpers.completeTotpMfaIfNeeded(client, {authBase, rl, deviceId, payload, totpSecret, referer: page.finalUrl});
  const workspaceId = helpers.pickWorkspaceId(payload);
  if (workspaceId) {
    const selected = await helpers.authJsonStep(client, authBase, 'POST', '/api/accounts/workspace/select',
      {workspace_id: workspaceId}, {referer: helpers.getContinueUrl(payload) || auth.toString()});
    payload = selected.data;
  } else {
    payload = await helpers.selectChatgptLoginWorkspaceIfNeeded(client, {authBase, payload});
  }
  const continued = await helpers.continueFlow(client, payload);
  client.bpsConnection = {callbackUrl: validateCallback(continued?.finalUrl || helpers.getContinueUrl(payload), auth.searchParams.get('state')), state: auth.searchParams.get('state'), codeVerifier: 'server-owned'};
  return {deviceId, emailVerified: true, loginMethod: 'password'};
}
