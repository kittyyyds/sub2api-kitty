<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Group, GroupPlatform } from '@/types'
import { getKeyGroupProvider, type KeyGroupProvider } from '@/utils/keyGroupProviders'

type GroupCategory = KeyGroupProvider

interface Props {
  modelValue: number | null
  groups: Group[]
  userRates?: Record<number, number>
  inputId?: string
  accessibleLabel?: string
  showPlatformTabs?: boolean
}

interface Emits {
  'update:modelValue': [value: number]
}

interface CategoryItem {
  id: GroupCategory
  label: string
  platform?: GroupPlatform
}

const props = withDefaults(defineProps<Props>(), {
  userRates: () => ({}),
  showPlatformTabs: true
})
const emit = defineEmits<Emits>()
const { t } = useI18n()

const isOpen = shallowRef(false)
const activeCategory = shallowRef<GroupCategory>('openai')
const searchQuery = shallowRef('')
const triggerRef = shallowRef<HTMLButtonElement | null>(null)
const panelRef = shallowRef<HTMLElement | null>(null)
const searchInputRef = shallowRef<HTMLInputElement | null>(null)
const panelStyle = shallowRef<Record<string, string>>({})

const categories = computed<CategoryItem[]>(() => [
  { id: 'openai', label: t('keys.groupPicker.openai'), platform: 'openai' },
  { id: 'anthropic', label: t('keys.groupPicker.claude'), platform: 'anthropic' },
  { id: 'gemini', label: t('keys.groupPicker.gemini'), platform: 'gemini' },
  { id: 'grok', label: t('keys.groupPicker.grok'), platform: 'grok' },
  { id: 'domestic', label: t('keys.providers.domestic'), platform: 'deepseek' },
  { id: 'video', label: t('keys.providers.video'), platform: 'video' },
  { id: 'other', label: t('keys.groupPicker.other') }
])

const getCategory = (platform: GroupPlatform): GroupCategory => {
  return getKeyGroupProvider(platform)
}

const selectedGroup = computed(() => props.groups.find((group) => group.id === props.modelValue) ?? null)

const visibleGroups = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return props.groups.filter((group) => {
    if (props.showPlatformTabs && getCategory(group.platform) !== activeCategory.value) return false
    if (!query) return true
    return (
      group.name.toLowerCase().includes(query) ||
      group.description?.toLowerCase().includes(query)
    )
  })
})

const firstAvailableCategory = (): GroupCategory => {
  return categories.value.find((category) =>
    props.groups.some((group) => getCategory(group.platform) === category.id)
  )?.id ?? 'openai'
}

const syncCategoryToSelection = () => {
  activeCategory.value = selectedGroup.value
    ? getCategory(selectedGroup.value.platform)
    : firstAvailableCategory()
}

const updatePanelPosition = () => {
  if (!isOpen.value || !triggerRef.value) return

  const margin = 12
  const gap = 6
  const targetHeight = 430
  const rect = triggerRef.value.getBoundingClientRect()
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  const width = Math.min(Math.max(320, rect.width), viewportWidth - margin * 2)
  const left = Math.min(
    Math.max(margin, rect.left),
    Math.max(margin, viewportWidth - width - margin)
  )
  const spaceBelow = viewportHeight - rect.bottom - gap - margin
  const spaceAbove = rect.top - gap - margin
  const opensBelow = spaceBelow >= Math.min(targetHeight, 300) || spaceBelow >= spaceAbove
  const availableHeight = Math.max(160, Math.min(targetHeight, opensBelow ? spaceBelow : spaceAbove))

  panelStyle.value = {
    left: `${left}px`,
    width: `${width}px`,
    maxHeight: `${availableHeight}px`,
    ...(opensBelow
      ? { top: `${rect.bottom + gap}px` }
      : { bottom: `${viewportHeight - rect.top + gap}px` })
  }
}

const open = async () => {
  syncCategoryToSelection()
  searchQuery.value = ''
  isOpen.value = true
  await nextTick()
  updatePanelPosition()
  searchInputRef.value?.focus()
}

const close = () => {
  isOpen.value = false
  searchQuery.value = ''
}

const toggle = () => {
  if (isOpen.value) {
    close()
    return
  }
  void open()
}

const chooseCategory = (category: GroupCategory) => {
  activeCategory.value = category
}

const chooseGroup = (group: Group) => {
  emit('update:modelValue', group.id)
  close()
  triggerRef.value?.focus()
}

const categoryTabClass = (category: GroupCategory) => {
  if (category !== activeCategory.value) {
    return 'text-gray-500 hover:bg-white hover:text-gray-800 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100'
  }

  const activeClasses: Record<GroupCategory, string> = {
    openai: 'bg-white text-emerald-700 shadow-sm ring-1 ring-emerald-200 dark:bg-emerald-900/20 dark:text-emerald-300 dark:ring-emerald-800',
    anthropic: 'bg-white text-amber-700 shadow-sm ring-1 ring-amber-200 dark:bg-amber-900/20 dark:text-amber-300 dark:ring-amber-800',
    gemini: 'bg-white text-sky-700 shadow-sm ring-1 ring-sky-200 dark:bg-sky-900/20 dark:text-sky-300 dark:ring-sky-800',
    grok: 'bg-white text-zinc-800 shadow-sm ring-1 ring-zinc-300 dark:bg-zinc-800 dark:text-zinc-100 dark:ring-zinc-600',
    domestic: 'bg-white text-teal-700 shadow-sm ring-1 ring-teal-200 dark:bg-teal-900/20 dark:text-teal-300 dark:ring-teal-800',
    video: 'bg-white text-cyan-700 shadow-sm ring-1 ring-cyan-200 dark:bg-cyan-900/20 dark:text-cyan-300 dark:ring-cyan-800',
    other: 'bg-white text-violet-700 shadow-sm ring-1 ring-violet-200 dark:bg-violet-900/20 dark:text-violet-300 dark:ring-violet-800'
  }
  return activeClasses[category]
}

const handleDocumentClick = (event: MouseEvent) => {
  if (!isOpen.value) return
  const target = event.target as Node
  if (triggerRef.value?.contains(target) || panelRef.value?.contains(target)) return
  close()
}

const handleDocumentKeydown = (event: KeyboardEvent) => {
  if (!isOpen.value || event.key !== 'Escape') return
  event.preventDefault()
  close()
  triggerRef.value?.focus()
}

watch(
  () => props.modelValue,
  () => {
    if (isOpen.value) syncCategoryToSelection()
  }
)

watch(
  () => props.groups,
  () => {
    const activeHasGroups = props.groups.some(
      (group) => getCategory(group.platform) === activeCategory.value
    )
    if (!activeHasGroups) activeCategory.value = firstAvailableCategory()
  }
)

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('resize', updatePanelPosition)
  window.addEventListener('scroll', updatePanelPosition, { capture: true, passive: true })
})

onUnmounted(() => {
  document.removeEventListener('click', handleDocumentClick)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', updatePanelPosition)
  window.removeEventListener('scroll', updatePanelPosition, { capture: true })
})
</script>

<template>
  <div class="group-picker">
    <button
      ref="triggerRef"
      :id="inputId"
      :aria-label="accessibleLabel"
      type="button"
      class="group-picker-trigger"
      :class="isOpen && 'group-picker-trigger-open'"
      aria-haspopup="listbox"
      :aria-expanded="isOpen"
      @click="toggle"
      @keydown.down.prevent="open"
      @keydown.up.prevent="open"
    >
      <span class="min-w-0 flex-1 text-left">
        <GroupBadge
          v-if="selectedGroup"
          :name="selectedGroup.name"
          :platform="selectedGroup.platform"
          :subscription-type="selectedGroup.subscription_type"
          :rate-multiplier="selectedGroup.rate_multiplier"
          :user-rate-multiplier="userRates[selectedGroup.id] ?? null"
          :peak-rate-enabled="selectedGroup.peak_rate_enabled"
          :peak-start="selectedGroup.peak_start"
          :peak-end="selectedGroup.peak_end"
          :peak-rate-multiplier="selectedGroup.peak_rate_multiplier"
          class="max-w-full"
        />
        <span v-else class="text-sm text-gray-400 dark:text-gray-500">
          {{ t('keys.selectGroup') }}
        </span>
      </span>
      <Icon
        name="chevronDown"
        size="md"
        :class="['shrink-0 text-gray-400 transition-transform duration-200', isOpen && 'rotate-180']"
      />
    </button>

    <Teleport to="body">
      <Transition name="group-picker-popover">
        <section
          v-if="isOpen"
          ref="panelRef"
          class="group-picker-panel"
          :style="panelStyle"
          @click.stop
          @keydown.esc.stop.prevent="handleDocumentKeydown"
        >
          <div class="group-picker-search">
            <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="search"
              class="group-picker-search-input"
              :placeholder="t('keys.searchGroup')"
            />
          </div>

          <div v-if="showPlatformTabs" class="group-picker-tabs" role="tablist" :aria-label="t('keys.groupPicker.platforms')">
            <button
              v-for="category in categories"
              :key="category.id"
              type="button"
              role="tab"
              :data-category="category.id"
              :aria-selected="activeCategory === category.id"
              :class="['group-picker-tab', categoryTabClass(category.id)]"
              @click="chooseCategory(category.id)"
            >
              <PlatformIcon v-if="category.platform" :platform="category.platform" size="sm" />
              <Icon v-else name="grid" size="sm" />
              <span class="truncate">{{ category.label }}</span>
            </button>
          </div>

          <div class="group-picker-list" role="listbox" :aria-label="t('keys.groupLabel')">
            <button
              v-for="group in visibleGroups"
              :key="group.id"
              type="button"
              role="option"
              :aria-selected="group.id === modelValue"
              :data-group-id="group.id"
              :class="[
                'group-picker-option',
                group.id === modelValue && 'group-picker-option-selected'
              ]"
              @click="chooseGroup(group)"
            >
              <GroupOptionItem
                :name="group.name"
                :platform="group.platform"
                :subscription-type="group.subscription_type"
                :rate-multiplier="group.rate_multiplier"
                :user-rate-multiplier="userRates[group.id] ?? null"
                :peak-rate-enabled="group.peak_rate_enabled"
                :peak-start="group.peak_start"
                :peak-end="group.peak_end"
                :peak-rate-multiplier="group.peak_rate_multiplier"
                :description="group.description"
                :selected="group.id === modelValue"
              />
            </button>

            <div v-if="visibleGroups.length === 0" class="group-picker-empty">
              <Icon name="inbox" size="lg" class="text-gray-300 dark:text-gray-600" />
              <span>{{ searchQuery ? t('keys.noGroupFound') : t('keys.groupPicker.empty') }}</span>
            </div>
          </div>
        </section>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.group-picker-trigger {
  @apply flex min-h-11 w-full items-center justify-between gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-900 transition-all duration-200;
  @apply hover:border-gray-300 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
  @apply dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100 dark:hover:border-dark-500;
}

.group-picker-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
}
</style>

<style>
.group-picker-panel {
  @apply fixed z-[100000020] flex flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-xl shadow-black/10;
  @apply dark:border-dark-700 dark:bg-dark-800 dark:shadow-black/30;
  pointer-events: auto !important;
}

.group-picker-search {
  @apply flex min-h-10 shrink-0 items-center gap-2 border-b border-gray-100 px-3 py-2 dark:border-dark-700;
}

.group-picker-search-input {
  @apply min-w-0 flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none;
  @apply dark:text-gray-100 dark:placeholder:text-gray-500;
}

.group-picker-tabs {
  @apply m-2 grid min-h-10 shrink-0 grid-cols-5 gap-1 rounded-lg bg-gray-50 p-1 dark:bg-dark-900/70;
}

.group-picker-tab {
  @apply flex min-h-8 min-w-0 items-center justify-center gap-1.5 rounded-md px-2 text-[13px] font-medium transition-colors;
  @apply focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40;
}

.group-picker-list {
  @apply min-h-0 flex-1 overflow-y-auto border-t border-gray-100 p-1.5 dark:border-dark-700;
}

.group-picker-option {
  @apply flex w-full items-start rounded-md border-b border-gray-100 px-3 py-2.5 text-left transition-colors last:border-b-0;
  @apply hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500/40;
  @apply dark:border-dark-700 dark:hover:bg-dark-700;
}

.group-picker-option-selected {
  @apply bg-primary-50 dark:bg-primary-900/20;
}

.group-picker-empty {
  @apply flex min-h-28 flex-col items-center justify-center gap-2 px-4 text-center text-sm text-gray-400 dark:text-gray-500;
}

.group-picker-popover-enter-active,
.group-picker-popover-leave-active {
  transition: opacity 150ms ease, transform 150ms ease;
}

.group-picker-popover-enter-from,
.group-picker-popover-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

@media (max-width: 639px) {
  .group-picker-tab {
    @apply flex-col gap-0.5 px-1 py-1 text-[11px];
  }

  .group-picker-option {
    @apply px-2 py-2;
  }
}

@media (prefers-reduced-motion: reduce) {
  .group-picker-popover-enter-active,
  .group-picker-popover-leave-active {
    transition: none;
  }
}
</style>
