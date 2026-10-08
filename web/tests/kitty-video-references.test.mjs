import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import { webcrypto } from 'node:crypto';
const require = createRequire(import.meta.url);
const ts = require('typescript');
const code = ts.transpileModule(readFileSync(new URL('../src/services/api/kitty-video-references.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText;

function fixture({ blob = new Blob(['fixture'], { type: 'image/png' }), changeUser = false } = {}) {
    let token = 'fixture-session';
    const requests = [];
    const exports = {};
    const modules = {
        '@/lib/kitty': { assertKittySession: () => ({ user: { id: 1 } }), kittyToken: () => token, kittyHeaders: () => ({ Authorization: `Bearer ${token}`, 'X-Canvas-Key-ID': '71' }) },
        '@/services/file-storage': { getMediaBlob: async () => { if (changeUser) token = 'changed'; return blob; } },
    };
    vm.runInNewContext(code, { exports, require: id => modules[id], URL, Blob, FormData, AbortController, setTimeout, clearTimeout, crypto: webcrypto,
        fetch: async (url, options) => { requests.push({ url, options }); return { ok: true, json: async () => ({ url: 'https://media.example/reference', expires_at: new Date(Date.now() + 86400000).toISOString() }) }; } });
    return { ...exports, requests };
}

test('public reference URLs are left unchanged without upload', async () => {
    const f = fixture();
    assert.equal(await f.kittyVideoReferenceURL({}, { url: 'https://media.example/public.png' }, 'image'), 'https://media.example/public.png');
    assert.equal(f.requests.length, 0);
});
test('selected local blob uses owner session and selected Key ID, never a raw API key or generation endpoint', async () => {
    const f = fixture();
    assert.equal(await f.kittyVideoReferenceURL({}, { storageKey: 'local-1', name: 'image.png' }, 'image'), 'https://media.example/reference');
    assert.equal(f.requests.length, 1);
    assert.equal(f.requests[0].url, '/canvas-api/v1/video-references');
    assert.equal(f.requests[0].options.headers['X-Canvas-Key-ID'], '71');
    assert.equal(f.requests[0].options.body.get('file').name, 'image.png');
    await f.kittyVideoReferenceURL({}, { storageKey: 'local-1', name: 'image.png' }, 'image');
    assert.equal(f.requests.length, 1, 'same file and key should reuse its unexpired temporary URL');
});
test('session changes while resolving local material stop upload', async () => {
    const f = fixture({ changeUser: true });
    await assert.rejects(f.kittyVideoReferenceURL({}, { storageKey: 'local-1' }, 'image'), /账号已变化/);
    assert.equal(f.requests.length, 0);
});
test('invalid local types and missing blobs never make network requests', async () => {
    for (const blob of [null, new Blob(['<svg/>'], { type: 'image/svg+xml' }), new Blob(['audio'], { type: 'audio/mpeg' })]) {
        const f = fixture({ blob });
        await assert.rejects(f.kittyVideoReferenceURL({}, { storageKey: 'local-1' }, 'image'));
        assert.equal(f.requests.length, 0);
    }
});
