import { assertKittySession, kittyHeaders, kittyToken } from '@/lib/kitty';
import { getMediaBlob } from '@/services/file-storage';
import type { AiConfig } from '@/stores/use-config-store';

type Reference = { url?: string; dataUrl?: string; storageKey?: string; name?: string };
const uploads = new Map<string, { url: string; expires: number }>();
let cachedOwner = 0;

// Public references remain URLs. Only explicitly selected local references are
// temporarily uploaded; the project and source blobs stay in local storage.
export async function kittyVideoReferenceURL(config: AiConfig, reference: Reference, kind: 'image' | 'audio' | 'video'): Promise<string> {
    const token = kittyToken();
    const owner = assertKittySession().user.id;
    if (cachedOwner !== owner) { uploads.clear(); cachedOwner = owner; }
    const headers = kittyHeaders(config);
    const source = reference.url || reference.dataUrl || '';
    if (/^https?:\/\//i.test(source)) {
        const url = new URL(source);
        if (url.username || url.password) throw new Error('参考素材链接应无需登录。');
        return source;
    }
    let blob = reference.storageKey ? await getMediaBlob(reference.storageKey) : null;
    if (!blob && /^(blob:|data:)/.test(source)) blob = await (await fetch(source)).blob();
    if (!blob || !blob.size) throw new Error('参考素材在本机不可用，请重新选择文件。');
    if (blob.size > 50 * 1024 * 1024) throw new Error('单个参考素材最多 50MB。');
    const accepted = kind === 'video' ? /^video\/mp4$/ : kind === 'image' ? /^image\/(png|jpeg|gif|webp)$/ : /^(audio\/(mpeg|mp3|wav|wave|x-wav|ogg|flac|x-flac|mp4|x-m4a)|application\/ogg)$/;
    if (!accepted.test(blob.type)) throw new Error(kind === 'video' ? '参考视频支持 MP4。' : kind === 'image' ? '参考图片支持 PNG、JPEG、GIF、WebP。' : '参考音频支持 MP3、WAV、OGG、FLAC、M4A。');
    const hash = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())), value => value.toString(16).padStart(2, '0')).join('');
    const identity = `${owner}:${headers['X-Canvas-Key-ID']}:${hash}`;
    if (kittyToken() !== token) throw new Error('登录账号已变化，请重新打开画布。');
    const cached = uploads.get(identity);
    if (cached && cached.expires > Date.now() + 60_000) return cached.url;
    const body = new FormData();
    body.append('file', blob, reference.name || `reference.${kind === 'image' ? 'png' : 'mp3'}`);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(new Error('素材上传超时，可重新提交。')), 90_000);
    try {
        const response = await fetch('/canvas-api/v1/video-references', { method: 'POST', headers, body, signal: controller.signal });
        const payload = await response.json().catch(() => null);
        if (!response.ok) throw new Error(payload?.error?.message || `素材上传失败（${response.status}）`);
        if (kittyToken() !== token) throw new Error('登录账号已变化，请重新打开画布。');
        if (typeof payload?.url !== 'string' || !payload.url.startsWith('https://') || !payload.expires_at) throw new Error('素材上传结果无效。');
        if (uploads.size >= 200) uploads.delete(uploads.keys().next().value!);
        uploads.set(identity, { url: payload.url, expires: Date.parse(payload.expires_at) });
        return payload.url;
    } finally { clearTimeout(timer); }
}
