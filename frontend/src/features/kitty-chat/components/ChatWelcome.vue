<script setup lang="ts">
defineProps<{
  logo: string
}>()

defineEmits<{
  prompt: [value: string]
}>()

const suggestions = [
  ['帮我写一个 Python 快速排序', '并解释时间复杂度'],
  ['把这段话润色得更专业', '贴上你的中文文案'],
  ['用通俗语言解释一个概念', '例如什么是 Prompt Caching'],
  ['写一封商务邮件', '说明合作意向与下一步'],
]
</script>

<template>
  <section class="chat-welcome" aria-labelledby="chat-welcome-title">
    <img :src="logo || '/logo.svg'" alt="站点 Logo">
    <h1 id="chat-welcome-title">网页直接试聊</h1>
    <p>选好 Key 与模型，消息会通过当前站点的兼容接口实时流式返回。</p>
    <div class="suggestion-grid">
      <button v-for="item in suggestions" :key="item[0]" type="button" @click="$emit('prompt', item[0])">
        <strong>{{ item[0] }}</strong>
        <span>{{ item[1] }}</span>
      </button>
    </div>
  </section>
</template>

<style scoped>
.chat-welcome {
  display: flex;
  width: min(610px, calc(100% - 32px));
  min-height: 430px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  margin: auto;
  text-align: center;
}

.chat-welcome img {
  width: 66px;
  height: 66px;
  border-radius: 8px;
  object-fit: contain;
  filter: drop-shadow(0 12px 22px rgb(15 118 110 / 20%));
}

.chat-welcome h1 {
  margin: 20px 0 0;
  color: var(--workbench-ink);
  font-size: 22px;
  font-weight: 780;
  letter-spacing: 0;
}

.chat-welcome p {
  max-width: 520px;
  margin: 12px 0 0;
  color: var(--workbench-muted);
  font-size: 12px;
  line-height: 1.7;
}

.suggestion-grid {
  display: grid;
  width: 100%;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
  margin-top: 24px;
}

.suggestion-grid button {
  min-height: 76px;
  border: 1px solid var(--workbench-line);
  border-radius: 7px;
  padding: 13px;
  background: color-mix(in srgb, var(--workbench-surface) 85%, transparent);
  text-align: left;
  transition: border-color .18s ease, background-color .18s ease, transform .18s ease;
}

.suggestion-grid button:hover {
  border-color: var(--workbench-accent);
  background: var(--workbench-accent-soft);
  transform: translateY(-2px);
}

.suggestion-grid strong,
.suggestion-grid span {
  display: block;
}

.suggestion-grid strong {
  color: var(--workbench-ink);
  font-size: 11px;
  font-weight: 700;
}

.suggestion-grid span {
  margin-top: 7px;
  color: var(--workbench-muted);
  font-size: 9px;
}

@media (max-width: 560px) {
  .suggestion-grid { grid-template-columns: 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .suggestion-grid button { transition: none; }
  .suggestion-grid button:hover { transform: none; }
}
</style>
