// Reviewer presets are independent of upstream account types.
export const REVIEWER_PROVIDER_PRESETS = [
 {id: 'mimo', labelKey: 'mimo', baseUrl: 'https://api.xiaomimimo.com/v1', models: ['mimo-v2.5']},
 {id: 'zhipu_glm', labelKey: 'zhipuGlm', baseUrl: 'https://open.bigmodel.cn/api/paas/v4', models: ['glm-4.7-flash', 'glm-4.7-flashx']},
 {id: 'alibaba_qwen', labelKey: 'alibabaQwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', models: ['qwen3.7-flash']},
] as const
