<script setup lang="ts">
import ModelIcon from '@/components/common/ModelIcon.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['models']
}

defineProps<Props>()

const models = [
  ['GPT', 'gpt-5.6'], ['Claude', 'claude-sonnet'], ['Gemini', 'gemini-2.5'], ['Grok', 'grok-4.5'],
  ['DeepSeek', 'deepseek-v3'], ['Qwen', 'qwen-max'], ['Kimi', 'kimi-k2'], ['MiniMax', 'minimax-m2'],
  ['Doubao', 'doubao-pro'], ['Llama', 'llama-4'], ['Mistral', 'mistral-large'], ['Cohere', 'command-r'],
  ['Yi', 'yi-large'], ['Perplexity', 'perplexity-sonar'], ['Jina', 'jina-embeddings'], ['OpenRouter', 'openrouter'],
  ['Suno', 'suno-v4'], ['Ollama', 'ollama'], ['Cloudflare', '@cf/model'], ['Midjourney', 'midjourney'],
  ['GLM', 'glm-4.5'], ['DALL-E', 'dall-e-3'], ['Imagen', 'imagen-3'], ['Veo', 'veo-3'],
]
</script>

<template>
  <section id="hr-models" class="hr-models hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-models-shell">
      <header>
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
      </header>

      <div class="hr-model-grid">
        <div v-for="([name, model]) in models" :key="name" class="hr-model-cell">
          <ModelIcon :model="model" size="28px" />
          <strong>{{ name }}</strong>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-models {
  background:
    radial-gradient(circle at 6% 14%, rgba(255, 159, 122, 0.18), transparent 28%),
    radial-gradient(circle at 90% 4%, rgba(109, 175, 200, 0.16), transparent 30%),
    var(--hr-bg);
}

.hr-models-shell {
  display: grid;
  gap: 54px;
  text-align: center;
}

.hr-models header {
  display: grid;
  justify-items: center;
  gap: 18px;
}

.hr-models h2 {
  max-width: 16ch;
  margin: 0;
  font-size: 4rem;
  font-weight: 750;
  letter-spacing: 0;
  line-height: 0.98;
  text-wrap: balance;
}

.hr-models p {
  margin: 0;
  color: var(--hr-muted);
  font-size: 17px;
}

.hr-model-grid {
  display: grid;
  overflow: hidden;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  border: 1px solid var(--hr-line);
  border-radius: 8px;
  background: var(--hr-panel-soft);
  box-shadow: 0 24px 50px var(--hr-shadow-soft);
}

.hr-model-cell {
  display: flex;
  min-width: 0;
  min-height: 104px;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border-right: 1px solid var(--hr-line);
  border-bottom: 1px solid var(--hr-line);
  padding: 14px 10px;
}

.hr-model-cell:nth-child(8n) {
  border-right: 0;
}

.hr-model-cell:nth-last-child(-n + 8) {
  border-bottom: 0;
}

.hr-model-cell strong {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  font-weight: 680;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .hr-models-shell {
    gap: 36px;
  }

  .hr-models header {
    justify-items: start;
    text-align: left;
  }

  .hr-models h2 {
    max-width: 12ch;
    font-size: 2.4rem;
  }

  .hr-model-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .hr-model-cell {
    min-height: 90px;
    flex-direction: column;
    border-right: 1px solid var(--hr-line);
    border-bottom: 1px solid var(--hr-line);
  }

  .hr-model-cell:nth-child(8n) {
    border-right: 1px solid var(--hr-line);
  }

  .hr-model-cell:nth-child(3n) {
    border-right: 0;
  }

  .hr-model-cell:nth-child(n + 13) {
    display: none;
  }

  .hr-model-cell:nth-last-child(-n + 8) {
    border-bottom: 1px solid var(--hr-line);
  }

  .hr-model-cell:nth-child(n + 10) {
    border-bottom: 0;
  }
}
</style>
