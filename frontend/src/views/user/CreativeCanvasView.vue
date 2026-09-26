<template>
  <AppLayout>
    <div class="card mx-auto max-w-3xl space-y-5 p-8">
      <p class="text-sm font-medium text-primary-600">KITTY CREATIVE</p>
      <h1 class="text-3xl font-semibold text-gray-900 dark:text-white">无限画布</h1>
      <p class="leading-7 text-gray-600 dark:text-gray-300">把灵感、图片与视频放在一张画布上。使用本站账号和已有 API 密钥，为不同创作步骤选择模型与线路。</p>
      <div class="rounded-xl bg-gray-50 p-4 text-sm leading-7 text-gray-600 dark:bg-dark-700 dark:text-gray-300">
        项目与素材保存在当前浏览器。请定期导出项目备份；切换设备或清理浏览器数据后，本地内容不会自动恢复。调用模型仍按所选密钥的分组价格计费。
      </div>
      <div class="flex flex-wrap gap-3">
        <a class="btn btn-primary" href="/canvas/canvas">进入无限画布</a>
        <router-link class="btn btn-secondary" to="/video-tasks">查看视频任务</router-link>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
</script>
