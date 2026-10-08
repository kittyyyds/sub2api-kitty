import type { KittyCapability } from './kitty';

export function buildUnifiedVideoRequest(cap: KittyCapability, input: { model: string; prompt: string; seconds: number; resolution: string; ratio: string; images: string[]; videos: string[]; audios: string[]; generateAudio?: boolean; face?: boolean }): Record<string, unknown> {
    if (!cap.supported || cap.family !== 'unified') throw new Error('当前型号尚未完成统一视频适配');
    if (!input.prompt.trim()) throw new Error('请输入视频提示词');
    if (!Number.isInteger(input.seconds) || input.seconds < (cap.seconds?.min || 1) || input.seconds > (cap.seconds?.max || 120) || (cap.second_options?.length && !cap.second_options.includes(input.seconds))) throw new Error('请选择当前型号支持的视频时长');
    if (!cap.resolutions?.includes(input.resolution) || !cap.aspect_ratios?.includes(input.ratio)) throw new Error('请选择当前型号支持的分辨率和比例');
    for (const [name, urls, max] of [['图片', input.images, cap.reference_images?.max || 0], ['视频', input.videos, cap.reference_videos?.max || 0], ['音频', input.audios, cap.reference_audios?.max || 0]] as const) {
        if (urls.length > max) throw new Error(`参考${name}最多 ${max} 个`);
        for (const value of urls) { const u = new URL(value); if (!['https:', 'http:'].includes(u.protocol) || u.username || u.password) throw new Error('请上传本地素材或填写公开链接'); }
    }
    if (input.images.length < (cap.reference_images?.min || 0)) throw new Error('当前型号需要参考图片');
    return { model: input.model, prompt: input.prompt.trim(), seconds: input.seconds, resolution: input.resolution, aspect_ratio: input.ratio,
        ...(input.images.length ? { images: input.images } : {}), ...(input.videos.length ? { videos: input.videos } : {}), ...(input.audios.length ? { audios: input.audios } : {}),
        ...(cap.generate_audio && input.generateAudio !== undefined ? { generate_audio: input.generateAudio } : {}), ...(cap.face && input.face !== undefined ? { face: input.face } : {}) };
}

export function kittyResolution(value: string, cap: KittyCapability): string {
    const normalized = /^\d+$/.test(value) ? value + 'p' : value.toLowerCase();
    return cap.resolutions?.length === 1 ? cap.resolutions[0] : normalized;
}
