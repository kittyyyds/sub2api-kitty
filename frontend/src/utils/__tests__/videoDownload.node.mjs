import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import vm from "node:vm";
const require = createRequire(import.meta.url);
const ts = require("typescript");
const encoder = new TextEncoder();
function fixture(initial = "") {
    let data = encoder.encode(initial), modified = 1, writes = 0, aborts = 0;
    const handle = { name: "task.mp4", getFile: async () => ({ size: data.length, lastModified: modified }), createWritable: async () => {
        let draft = data.slice(), position = 0;
        const truncate = async (length) => { const next = new Uint8Array(length); next.set(draft.subarray(0, length)); draft = next; };
        return { truncate, seek: async (value) => { position = value; }, write: async (chunk) => { writes++; if (position + chunk.length > draft.length) await truncate(position + chunk.length); draft.set(chunk, position); position += chunk.length; }, close: async () => { data = draft; modified++; }, abort: async () => { aborts++; } };
    } };
    const exports = {};
    const code = ts.transpileModule(readFileSync(new URL("../videoDownload.ts", import.meta.url), "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText;
    vm.runInNewContext(code, { exports, window: { showSaveFilePicker: async () => handle }, setTimeout, clearTimeout, AbortController, DOMException, Uint8Array, Response });
    return { api: exports, handle, text: () => new TextDecoder().decode(data), edit: (value) => { data = encoder.encode(value); modified++; }, counts: () => ({ writes, aborts }) };
}
function response(chunks, headers = {}, status = 200, failure) {
    let index = 0;
    return new Response(new ReadableStream({ pull(controller) {
        if (index < chunks.length) controller.enqueue(encoder.encode(chunks[index++]));
        else if (failure === "stall") return new Promise(() => {});
        else if (failure) controller.error(new Error("network interrupted"));
        else controller.close();
    } }), { status, headers: { "content-type": "video/mp4", ...headers } });
}
async function ready(f) { return f.api.pickVideoDownloadFile("user:key:task", "task.mp4"); }
async function run(f, session, request, more = {}) { return f.api.downloadVideoToFile({ identity: "user:key:task", session, request, signal: new AbortController().signal, ...more }); }
async function partial(f, session, etag = '"v1"') {
    await assert.rejects(run(f, session, async () => response(["abc"], { "content-length": "6", etag })), /中断/);
    assert.equal(f.text(), "abc"); assert.equal(session.bytes, 3);
}

test("stream writes each chunk directly and finalizes an exact whole file", async () => {
    const f = fixture("older-longer-file"), session = await ready(f);
    const result = await run(f, session, async () => response(["abc", "def"], { "content-length": "6", etag: '"v1"' }));
    assert.equal(f.text(), "abcdef"); assert.equal(result.bytes, 6); assert.equal(session.complete, true); assert.equal(f.counts().writes, 2);
});
test("truncated strong-ETag response persists a partial file then resumes the same task", async () => {
    const f = fixture(), session = await ready(f); await partial(f, session);
    const result = await run(f, session, async (headers) => {
        assert.equal(headers.Range, "bytes=3-"); assert.equal(headers["If-Range"], '"v1"');
        return response(["def"], { "content-length": "3", "content-range": "bytes 3-5/6", etag: '"v1"' }, 206);
    });
    assert.equal(result.resumed, true); assert.equal(f.text(), "abcdef");
});
test("a 200 response to a range request replaces from byte zero, never appends", async () => {
    const f = fixture(), session = await ready(f); await partial(f, session);
    await run(f, session, async (headers) => { assert.equal(headers.Range, "bytes=3-"); return response(["123456"], { "content-length": "6", etag: '"v2"' }); });
    assert.equal(f.text(), "123456");
});
test("missing and weak ETags restart full downloads instead of range resume", async () => {
    for (const etag of ["", 'W/"v1"']) {
        const f = fixture(), session = await ready(f); await partial(f, session, etag);
        await run(f, session, async (headers) => { assert.equal(headers.Range, undefined); assert.equal(headers["If-Range"], undefined); return response(["123456"], { "content-length": "6" }); });
        assert.equal(f.text(), "123456");
    }
});
test("changed ETag and incorrect range offsets never corrupt the existing partial file", async () => {
    for (const headers of [{ "content-range": "bytes 3-5/6", etag: '"changed"' }, { "content-range": "bytes 2-5/6", etag: '"v1"' }, { "content-range": "bytes 3-5/7", etag: '"v1"' }]) {
        const f = fixture(), session = await ready(f); await partial(f, session);
        await assert.rejects(run(f, session, async () => response(["def"], headers, 206)), /分段/);
        assert.equal(f.text(), "abc"); assert.equal(session.etag, undefined);
    }
});
test("416 triggers at most one full GET and never marks a partial file complete", async () => {
    const f = fixture(), session = await ready(f); await partial(f, session); let calls = 0;
    await run(f, session, async (headers) => { calls++; if (calls === 1) return new Response(null, { status: 416, headers: { "content-range": "bytes */6" } }); assert.equal(headers.Range, undefined); return response(["abcdef"], { "content-length": "6", etag: '"v1"' }); });
    assert.equal(calls, 2); assert.equal(f.text(), "abcdef");
});
test("a second 416 stops without loops or an appended error document", async () => {
    const f = fixture(), session = await ready(f); await partial(f, session); let calls = 0;
    await assert.rejects(run(f, session, async () => { calls++; return new Response(null, { status: 416 }); }), /416/);
    assert.equal(calls, 2); assert.equal(f.text(), "abc"); assert.equal(session.complete, false);
});
test("cancel saves a bounded partial file and retry keeps the same task", async () => {
    const f = fixture(), session = await ready(f), controller = new AbortController();
    await assert.rejects(run(f, session, async () => response(["abc", "def"], { "content-length": "6", etag: '"v1"' }), { signal: controller.signal, onProgress: (p) => { if (p.received === 3) controller.abort(); } }));
    assert.equal(f.text(), "abc"); assert.equal(session.active, false); assert.equal(session.bytes, 3);
});
test("network errors preserve the confirmed written prefix", async () => {
    const f = fixture(), session = await ready(f);
    await assert.rejects(run(f, session, async () => response(["abc"], { "content-length": "6", etag: '"v1"' }, 200, "error")), /network/);
    assert.equal(f.text(), "abc"); assert.equal(session.bytes, 3);
});
test("response-header timeout is bounded and leaves the selected existing file untouched", async () => {
    const f = fixture("original"), session = await ready(f);
    await assert.rejects(run(f, session, () => new Promise(() => {}), { idleTimeoutMs: 5 }), /超时/);
    assert.equal(f.text(), "original"); assert.equal(session.active, false);
});
test("stalled body reads time out, saving written data for retry", async () => {
    const f = fixture(), session = await ready(f);
    await assert.rejects(run(f, session, async () => response(["abc"], { "content-length": "6", etag: '"v1"' }, 200, "stall"), { idleTimeoutMs: 5 }), /超时/);
    assert.equal(f.text(), "abc");
});
test("externally edited local files cannot be resumed by size alone", async () => {
    const f = fixture(), session = await ready(f); await partial(f, session); f.edit("xyz");
    await run(f, session, async (headers) => { assert.equal(headers.Range, undefined); return response(["123456"], { "content-length": "6" }); });
    assert.equal(f.text(), "123456");
});
test("oversized streams abort the new write instead of finalizing corrupt bytes", async () => {
    const f = fixture("original"), session = await ready(f);
    await assert.rejects(run(f, session, async () => response(["abcdefg"], { "content-length": "6" })), /超过/);
    assert.equal(f.text(), "original"); assert.equal(f.counts().aborts, 1); assert.equal(session.bytes, 0);
});
test("partial ranges and encoded/error responses are rejected before file writes", async () => {
    for (const [headers, status] of [[{ "content-range": "bytes 0-2/6" }, 206], [{ "content-type": "application/json" }, 200], [{ "content-encoding": "gzip" }, 200], [{ "content-length": "not-a-number" }, 200]]) {
        const f = fixture("original"), session = await ready(f);
        await assert.rejects(run(f, session, async () => response(["abc"], headers, status)));
        assert.equal(f.text(), "original"); assert.equal(f.counts().writes, 0);
    }
});
test("task identity and concurrent writers are rejected before any request", async () => {
    const f = fixture(), session = await ready(f); let calls = 0; const request = async () => { calls++; return response(["abc"]); };
    await assert.rejects(run(f, session, request, { identity: "other-task" }), /任务/);
    session.active = true; await assert.rejects(run(f, session, request), /正在下载/); assert.equal(calls, 0);
});
test("fully received unknown-length stream is valid but no ETag means no resume", async () => {
    const f = fixture(), session = await ready(f); const result = await run(f, session, async () => response(["abc"]));
    assert.equal(result.bytes, 3); assert.equal(session.complete, true); assert.equal(session.etag, undefined);
});
