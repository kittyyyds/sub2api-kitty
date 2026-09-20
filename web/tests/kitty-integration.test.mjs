import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import vm from "node:vm";

const require = createRequire(import.meta.url);
const ts = require("typescript");
function loadModule(file, globals = {}, modules = {}) {
    const exports = {};
    const code = ts.transpileModule(readFileSync(new URL(file, import.meta.url), "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true } }).outputText;
    vm.runInNewContext(code, { exports, require: (id) => modules[id] ?? require(id), process: { env: { NEXT_PUBLIC_KITTY_MODE: "true" } }, URL, Date, Map, Set, ...globals }, { filename: file });
    return exports;
}
const video = loadModule("../src/lib/kitty-suanli-video.ts");
const imageURL = "https://fixtures.example/reference.png";
const audioURL = "https://fixtures.example/reference.mp3";
const normalized = (value) => JSON.parse(JSON.stringify(value));

test("all nine video models build their documented JSON family without FormData", () => {
    assert.equal(video.SUANLI_VIDEO_MODELS.length, 9);
    for (const model of video.SUANLI_VIDEO_MODELS) {
        const body = video.buildSuanliVideoRequest({ modelID: model.id, prompt: "fixture", duration: 5, ratio: "16:9", imageURLs: [imageURL], audioURLs: model.family === "h3" ? [audioURL] : [] });
        assert.equal(body.model, model.id);
        if (model.family === "sd25") {
            assert.deepEqual(normalized(body), { model: model.id, prompt: "fixture", resolution: "720p", aspect_ratio: "16:9", images: [imageURL] });
            assert.equal("seconds" in body, false);
        } else {
            assert.equal(body.seconds, 5);
            assert.equal(body.size, video.suanliPixelSize(model.resolution, "16:9"));
            assert.deepEqual(normalized(body.reference_images), [{ url: imageURL, role: "reference_image" }]);
            if (model.family === "h3") assert.deepEqual(normalized(body.reference_audios), [{ url: audioURL }]);
        }
    }
});

test("video rejects unsupported model, duration, extra references and local blobs", () => {
    const input = { modelID: video.SUANLI_VIDEO_MODELS[0].id, prompt: "fixture", duration: 5, ratio: "1:1", imageURLs: [imageURL], audioURLs: [] };
    assert.throws(() => video.buildSuanliVideoRequest({ ...input, modelID: "unknown" }));
    assert.throws(() => video.buildSuanliVideoRequest({ ...input, duration: 16 }));
    assert.throws(() => video.buildSuanliVideoRequest({ ...input, imageURLs: [imageURL, imageURL] }));
    assert.throws(() => video.buildSuanliVideoRequest({ ...input, imageURLs: ["blob:local-file"] }));
    assert.throws(() => video.buildSuanliVideoRequest({ ...input, audioURLs: [audioURL] }));
});

function fixture(owner = 7) {
    const localStorage = new Map([["auth_token", "fixture-session"]]);
    const requests = [];
    const data = { user: { id: owner, username: "fixture", email: "fixture@example.test", balance: 1 }, keys: [{ id: 11, name: "primary", key_display: "sk-***11", status: "active", group_id: 1 }, { id: 12, name: "secondary", key_display: "sk-***12", status: "active", group_id: 1 }, { id: 13, name: "expired", key_display: "sk-***13", status: "active", group_id: 1, expires_at: "2000-01-01T00:00:00Z" }], storage_mode: "local", capabilities_version: "1" };
    const kitty = loadModule("../src/lib/kitty.ts", { window: { localStorage: { getItem: (key) => localStorage.get(key) || null } }, fetch: async (url, options) => { requests.push({ url, options }); return { ok: true, status: 200, json: async () => ({ code: 0, data: url.endsWith("bootstrap") ? data : { models: [{ id: "text-model", capabilities: { media_type: "text", supported: true } }, { id: "disabled-model", capabilities: { media_type: "audio", supported: false } }] } }) }; } });
    return { kitty, localStorage, requests, data };
}

test("bootstrap exposes masked Key channels and preserves explicit invalid selections", async () => {
    const { kitty, requests } = fixture();
    const channels = await kitty.loadKittyChannels();
    assert.equal(channels.length, 3);
    assert.equal(channels.every((channel) => channel.apiKey === ""), true);
    assert.deepEqual(normalized(channels[0].models), ["text-model"]);
    assert.equal(channels[2].models.length, 0);
    assert.equal(requests.some((request) => request.url.includes("/13/models")), false);
    const config = kitty.mergeKittyConfig({ textChannelId: "kitty-key-99", textModel: "text-model", apiKey: "old-secret" }, channels);
    assert.equal(config.textChannelId, "kitty-key-99");
    assert.equal(config.apiKey, "");
    assert.throws(() => kitty.kittyHeaders({ ...config, model: "text-model" }), /Key/);
    const headers = kitty.kittyHeaders({ ...config, model: "text-model", textChannelId: "kitty-key-12" });
    assert.equal(headers["X-Canvas-Key-ID"], "12");
    assert.equal(headers.Authorization, "Bearer fixture-session");
});

test("session changes stop both generation headers and local data access", async () => {
    const { kitty, localStorage } = fixture();
    await kitty.loadKittyChannels();
    assert.equal(await kitty.kittyStorageOwner(), "7");
    localStorage.set("auth_token", "different-session");
    assert.throws(() => kitty.kittyHeaders({ model: "text-model", textChannelId: "kitty-key-11" }), /会话/);
    await assert.rejects(kitty.kittyStorageOwner(), /会话/);
});

test("storage resolves bootstrap before selecting distinct per-user databases", async () => {
    const databases = new Map();
    const localforage = { createInstance(options) { const id = `${options.name}/${options.storeName}`; if (!databases.has(id)) databases.set(id, new Map()); const db = databases.get(id); return { async getItem(key) { return db.get(key) ?? null; }, async setItem(key, value) { db.set(key, value); return value; } }; } };
    const a = fixture(7), b = fixture(8);
    const moduleA = loadModule("../src/lib/user-localforage.ts", {}, { "localforage": localforage, "@/lib/kitty": a.kitty });
    const moduleB = loadModule("../src/lib/user-localforage.ts", {}, { "localforage": localforage, "@/lib/kitty": b.kitty });
    const storeA = moduleA.createUserLocalStore({ name: "infinite-canvas", storeName: "app_state" });
    const storeB = moduleB.createUserLocalStore({ name: "infinite-canvas", storeName: "app_state" });
    await storeA.setItem("project", "user-seven");
    assert.equal(await storeB.getItem("project"), null);
    assert.equal(await storeA.getItem("project"), "user-seven");
    assert.equal(databases.has("kitty-canvas-user-7/app_state"), true);
    assert.equal(databases.has("kitty-canvas-user-8/app_state"), true);
});

test("video download uses the task Key, verifies complete bytes and stores only after success", async () => {
    const { kitty } = fixture();
    await kitty.loadKittyChannels();
    const calls = [];
    let saved = 0;
    let length = "4";
    let status = 200;
    const module = loadModule("../src/services/api/kitty-video.ts", { AbortController, Blob, Uint8Array, setTimeout, clearTimeout, fetch: async (url, options) => {
        calls.push({ url, options });
        let offset = 0;
        return { ok: status >= 200 && status < 300, status, headers: new Map([["content-length", length], ["content-type", "video/mp4"]]), body: { getReader: () => ({ read: async () => offset++ === 0 ? { done: false, value: new Uint8Array([1, 2, 3, 4]) } : { done: true } }) } };
    } }, { "@/lib/kitty": kitty, "@/services/file-storage": { uploadMediaFile: async (blob) => { saved++; return { url: "blob:stored", storageKey: "kitty-video:fixture", bytes: blob.size }; } } });
    const task = { id: "task-one", kittyKeyId: "12" };
    const result = await module.downloadKittyVideo(task, new AbortController().signal, () => {});
    assert.equal(result.bytes, 4);
    assert.equal(saved, 1);
    assert.equal(calls[0].url, "/canvas-api/v1/videos/task-one/content");
    assert.equal(calls[0].options.headers["X-Canvas-Key-ID"], "12");
    length = "8";
    await assert.rejects(module.downloadKittyVideo(task, new AbortController().signal, () => {}), /不完整/);
    assert.equal(saved, 1);
    status = 206;
    await assert.rejects(module.downloadKittyVideo(task, new AbortController().signal, () => {}), /部分内容/);
    assert.equal(saved, 1);
    status = 410;
    await assert.rejects(module.downloadKittyVideo(task, new AbortController().signal, () => {}), /过期/);
    assert.equal(saved, 1);
    assert.equal(calls.every((call) => call.url.endsWith("/content")), true);
});

test("quote confirmation rejects failed, canceled or changed pricing", async () => {
    let price = 2, confirmations = 0, changed = false;
    const quote = loadModule("../src/services/api/kitty-quote.ts", {}, { "@/lib/kitty": { kittyHeaders: () => ({ "X-Canvas-Key-ID": "11" }), kittyCapability: () => ({ family: "seedance" }), kittyRequest: async () => ({ final_unit_price: true, unit_price: price, estimated_cost: price * 5, billing_mode: "video", seconds: 5, count: 1 }) } });
    quote.registerKittyQuoteConfirmation(async () => { confirmations++; if (changed) price = 3; return true; });
    await quote.confirmKittyVideoQuote({ model: "fixture", videoSeconds: "5" });
    assert.equal(confirmations, 1);
    changed = true;
    await assert.rejects(quote.confirmKittyVideoQuote({ model: "fixture", videoSeconds: "5" }), /报价已变化/);
    quote.registerKittyQuoteConfirmation(async () => false);
    await assert.rejects(quote.confirmKittyVideoQuote({ model: "fixture", videoSeconds: "5" }), /取消/);
    price = NaN;
    await assert.rejects(quote.fetchKittyVideoQuote({ model: "fixture", videoSeconds: "5" }), /报价无效/);
});

test("a successful HTTP create without task ID is unknown; simultaneous node creates share one POST", async () => {
    let calls = 0, missing = true;
    const modules = new Proxy({
        "axios": { post: async () => { calls++; return { data: missing ? { status: "queued" } : { id: "task-fixture", status: "queued" } }; }, isAxiosError: () => false },
        "@/lib/kitty": { KITTY_MODE: true, kittyHeaders: () => ({ "X-Canvas-Key-ID": "11" }) },
        "./kitty-quote": { confirmKittyVideoQuote: async () => {} },
        "@/lib/kitty-suanli-video": video,
        "@/lib/seedance-video": { normalizeSeedanceRatio: () => "16:9" },
        "@/lib/gemini": { isGeminiConfig: () => false, isGeminiVideoModel: () => false },
        "@/lib/minimax-video": { isMiniMaxH3Config: () => false },
        "@/lib/video-model-capabilities": { isCogVideoX3Model: () => false },
        "@/stores/use-config-store": { localChannelForActiveModel: () => ({ id: "kitty-key-11", protocol: "openai" }), directAIProviderForConfig: () => null, channelProtocolForConfig: () => "openai" },
        "@/stores/use-user-store": { useUserStore: { getState: () => ({ token: "" }) } },
    }, { get: (target, name) => target[name] ?? {} });
    const module = loadModule("../src/services/api/video.ts", { FormData }, modules);
    const config = { model: video.SUANLI_VIDEO_MODELS[0].id, videoSeconds: "5", systemPrompts: { video: "" }, systemPrompt: "", channelMode: "local" };
    const references = { kittyImageURLs: imageURL };
    await assert.rejects(module.createVideoGenerationTask(config, "fixture", references, undefined, { clientTaskId: "missing-id" }), (error) => error.submissionUnknown === true);
    assert.equal(calls, 1);
    missing = false;
    const results = await Promise.all([module.createVideoGenerationTask(config, "fixture", references, undefined, { clientTaskId: "same-node" }), module.createVideoGenerationTask(config, "fixture", references, undefined, { clientTaskId: "same-node" })]);
    assert.equal(calls, 2);
    assert.equal(results[0].task.id, "task-fixture");
    assert.equal(results[0].task.kittyKeyId, "11");
    assert.equal(results[1].task.id, "task-fixture");
});
