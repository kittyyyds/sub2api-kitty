<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'

import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'
import DocumentationPanel from './DocumentationPanel.vue'

const appStore = useAppStore()
const guideUrl = computed(() => sanitizeUrl(appStore.docUrl))
const isLoading = shallowRef(true)
watch(guideUrl, () => { isLoading.value = true })

function handleFrameLoad() {
  isLoading.value = false
}
</script>

<template>
  <DocumentationPanel v-if="!guideUrl" />
  <section v-else class="feishu-guide" aria-labelledby="feishu-guide-title">
    <header class="feishu-guide__toolbar">
      <div class="feishu-guide__heading">
        <span class="feishu-guide__icon" aria-hidden="true">
          <Icon name="book" size="sm" :stroke-width="1.8" />
        </span>
        <div>
          <h1 id="feishu-guide-title">新手必看</h1>
          <p>使用说明与接入文档</p>
        </div>
      </div>

      <a
        :href="guideUrl"
        class="btn btn-secondary btn-sm feishu-guide__open"
        target="_blank"
        rel="noopener noreferrer"
        data-testid="feishu-guide-open"
      >
        <Icon name="externalLink" size="sm" :stroke-width="2" />
        <span>新窗口打开</span>
      </a>
    </header>

    <div class="feishu-guide__frame-shell">
      <div v-if="isLoading" class="feishu-guide__loading" role="status" aria-live="polite">
        <span class="feishu-guide__spinner" aria-hidden="true"></span>
        <span>文档加载中...</span>
      </div>

      <iframe
        :src="guideUrl"
        class="feishu-guide__frame"
        title="Kitty API 新手必看文档"
        referrerpolicy="strict-origin-when-cross-origin"
        allowfullscreen
        data-testid="feishu-guide-frame"
        @load="handleFrameLoad"
      ></iframe>
    </div>
  </section>
</template>

<style scoped>
.feishu-guide {
  display: flex;
  width: 100%;
  min-width: 0;
  height: calc(100dvh - 6rem);
  min-height: 420px;
  overflow: hidden;
  background: var(--customer-surface, #ffffff);
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 8px;
  flex-direction: column;
}

.feishu-guide__toolbar {
  display: flex;
  flex: 0 0 auto;
  min-height: 56px;
  padding: 8px 12px 8px 14px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  background: var(--customer-surface, #ffffff);
  border-bottom: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.feishu-guide__heading {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.feishu-guide__icon {
  display: inline-grid;
  flex: 0 0 32px;
  width: 32px;
  height: 32px;
  color: var(--customer-cyan, #0a7f90);
  background: var(--customer-cyan-soft, rgba(10, 127, 144, 0.1));
  border-radius: 6px;
  place-items: center;
}

.feishu-guide__heading h1 {
  margin: 0;
  color: var(--customer-ink, #142126);
  font-size: 15px;
  font-weight: 680;
  letter-spacing: 0;
  line-height: 1.35;
}

.feishu-guide__heading p {
  margin: 2px 0 0;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 12px;
  line-height: 1.35;
}

.feishu-guide__open {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
}

.feishu-guide__frame-shell {
  position: relative;
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
  background: #ffffff;
}

.feishu-guide__loading {
  position: absolute;
  z-index: 1;
  inset: 0;
  display: flex;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 13px;
  align-items: center;
  justify-content: center;
  gap: 10px;
  background: var(--customer-surface, #ffffff);
}

.feishu-guide__spinner {
  width: 22px;
  height: 22px;
  border: 2px solid rgba(10, 127, 144, 0.22);
  border-top-color: var(--customer-cyan, #0a7f90);
  border-radius: 50%;
  animation: feishu-guide-spin 0.8s linear infinite;
}

.feishu-guide__frame {
  display: block;
  width: 100%;
  height: 100%;
  margin: 0;
  background: #ffffff;
  border: 0;
}

@keyframes feishu-guide-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (min-width: 768px) {
  .feishu-guide {
    height: calc(100dvh - 7rem);
  }
}

@media (min-width: 1024px) {
  .feishu-guide {
    height: calc(100dvh - 8rem);
  }
}

@media (max-width: 480px) {
  .feishu-guide__toolbar {
    min-height: 52px;
    padding: 8px;
  }

  .feishu-guide__heading p,
  .feishu-guide__open span {
    display: none;
  }

  .feishu-guide__open {
    width: 36px;
    height: 36px;
    padding: 0;
    justify-content: center;
  }
}

@media (prefers-reduced-motion: reduce) {
  .feishu-guide__spinner {
    animation-duration: 1.6s;
  }
}
</style>
