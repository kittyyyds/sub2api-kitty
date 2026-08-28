<script setup lang="ts">
import { useI18n } from "vue-i18n";

const enabled = defineModel<boolean>("enabled", { required: true });
const mode = defineModel<"prepend" | "append">("mode", { required: true });
const content = defineModel<string>("content", { required: true });

const { t } = useI18n();
</script>

<template>
  <section class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-700">
    <label class="flex cursor-pointer items-center gap-3">
      <input
        v-model="enabled"
        type="checkbox"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.groups.form.grokPromptEnabled") }}
      </span>
    </label>

    <template v-if="enabled">
      <div>
        <label class="input-label">{{ t("admin.groups.form.grokPromptMode") }}</label>
        <select v-model="mode" class="input">
          <option value="prepend">{{ t("admin.groups.form.grokPromptPrepend") }}</option>
          <option value="append">{{ t("admin.groups.form.grokPromptAppend") }}</option>
        </select>
      </div>

      <div>
        <label class="input-label">{{ t("admin.groups.form.grokPromptContent") }}</label>
        <textarea
          v-model="content"
          rows="9"
          maxlength="36864"
          class="input min-h-48 resize-y font-mono text-xs leading-5"
          :placeholder="t('admin.groups.form.grokPromptPlaceholder')"
        />
        <p class="input-hint">{{ t("admin.groups.form.grokPromptHint") }}</p>
      </div>
    </template>
  </section>
</template>
