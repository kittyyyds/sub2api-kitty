import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
export function fixture() {
  const model: PlazaModel = { name: '渠道/原始模型-ID', platform: 'video', description: '根据参考图生成视频',
    capabilities: { media_type: 'video', supported: true, seconds: { min: 5, max: 15 }, second_options: [5, 10, 15],
      reference_videos: { min: 0, max: 2 }, max_reference_seconds: 12, operations: ['image_to_video'] },
    official_pricing: null, pricing: { billing_mode: 'video', per_request_price: 0.3, final_unit_price: true, reference_per_second: 0.1,
      input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, image_input_price: null, image_output_price: null, intervals: [] } }
  const group: ModelPlazaGroup = { id: 51, name: '视频模型', description: '', platform: 'video', subscription_type: 'standard',
    rate_multiplier: 9, user_rate_multiplier: 7, image_rate_independent: false, image_rate_multiplier: 1, video_rate_independent: true, video_rate_multiplier: 5,
    peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1, is_exclusive: false, long_context_pricing_enabled: false, models: [model] }
  return { description: '', groups: [group] }
}
