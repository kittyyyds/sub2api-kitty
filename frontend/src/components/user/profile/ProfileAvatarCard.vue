<template>
  <div :class="props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.avatar.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.avatar.description') }}
      </p>
    </div>

    <div :class="props.embedded ? 'space-y-3' : 'flex flex-col gap-5 px-6 py-6 sm:flex-row sm:items-start'">
      <div
        :class="props.embedded
          ? 'flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-xl font-bold text-white shadow-lg shadow-primary-500/20'
          : 'flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-3xl font-bold text-white shadow-lg shadow-primary-500/20'"
      >
        <img
          v-if="avatarPreviewUrl && !avatarBroken"
          data-testid="profile-avatar-preview"
          :src="avatarPreviewUrl"
          @error="avatarBroken = true"
          :alt="displayName"
          class="h-full w-full object-cover"
        >
        <span v-else>{{ avatarInitial }}</span>
      </div>

      <div :class="props.embedded ? 'space-y-3' : 'min-w-0 flex-1 space-y-4'">
        <div class="space-y-1">
          <p v-if="props.embedded" class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.avatar.title') }}
          </p>
          <p v-else class="text-sm font-medium text-gray-900 dark:text-white">
            {{ displayName }}
          </p>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.avatar.uploadHint') }}
            {{ t('optimizations.autoSave') }}
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <label class="btn btn-secondary btn-sm cursor-pointer">
            <input
              data-testid="profile-avatar-file-input"
              type="file"
              accept="image/*"
              :disabled="avatarSaving"
              class="hidden"
              @change="handleAvatarFileChange"
            >
            {{ t('profile.avatar.uploadAction') }}
          </label>

          <span v-if="avatarSaving" role="status" class="text-sm text-gray-500">{{ t('common.saving') }}</span>

          <button
            data-testid="profile-avatar-delete"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="avatarSaving"
            @click="handleAvatarDelete"
          >
            {{ t('common.delete') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userAPI } from '@/api'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = withDefaults(defineProps<{
  user: User | null
  embedded?: boolean
}>(), {
  embedded: false,
})

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const targetAvatarUploadBytes = 20 * 1024
const avatarScaleSteps = [1, 0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36]
const avatarQualitySteps = [0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36]
const avatarDraft = ref('')
const avatarSaving = ref(false)
const avatarBroken = ref(false)
const savedAvatar = ref(props.user?.avatar_url?.trim() || '')
let operation = 0
let disposed = false
onBeforeUnmount(() => { disposed = true; operation++ })
watch(() => props.user?.id, () => { operation++; avatarSaving.value = false; avatarDraft.value = ''; savedAvatar.value = props.user?.avatar_url?.trim() || '' })

const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || t('profile.user'))
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')
const avatarPreviewUrl = computed(() => avatarDraft.value.trim() || savedAvatar.value)

watch(avatarPreviewUrl, () => { avatarBroken.value = false })

watch(
  () => props.user?.avatar_url,
  () => {
    savedAvatar.value = props.user?.avatar_url?.trim() || ''
  }
)

function normalizeUploadedAvatar(value: string): string | null {
  const normalized = value.trim()
  if (!normalized) {
    return null
  }

  if (!/^data:image\/[a-zA-Z0-9.+-]+;base64,/i.test(normalized)) {
    appStore.showError(t('profile.avatar.uploadRequired'))
    return null
  }

  return normalized
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(reader.error ?? new Error('avatar_read_failed'))
    reader.readAsDataURL(file)
  })
}

function loadImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(t('profile.avatar.readFailed')))
    image.src = dataURL
  })
}

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error(t('profile.avatar.compressFailed')))
        return
      }
      resolve(blob)
    }, type, quality)
  })
}

async function compressAvatarFile(file: File): Promise<File> {
  const sourceDataURL = await readFileAsDataURL(file)
  const image = await loadImage(sourceDataURL)
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    throw new Error(t('profile.avatar.compressFailed'))
  }

  for (const scale of avatarScaleSteps) {
    const width = Math.max(1, Math.round(Math.min(1, 256 / Math.max(image.naturalWidth, image.naturalHeight)) * image.naturalWidth * scale))
    const height = Math.max(1, Math.round(Math.min(1, 256 / Math.max(image.naturalWidth, image.naturalHeight)) * image.naturalHeight * scale))
    canvas.width = width
    canvas.height = height
    ctx.clearRect(0, 0, width, height)
    ctx.drawImage(image, 0, 0, width, height)

    for (const quality of avatarQualitySteps) {
      let blob = await canvasToBlob(canvas, 'image/webp', quality)
      // Safari may silently encode PNG for an unsupported WebP request.
      // Use the actual MIME, never label PNG bytes as WebP.
      if (blob.type !== 'image/webp') {
        ctx.save()
        ctx.globalCompositeOperation = 'destination-over'
        ctx.fillStyle = '#ffffff'
        ctx.fillRect(0, 0, width, height)
        ctx.restore()
        blob = await canvasToBlob(canvas, 'image/jpeg', quality)
      }
      if (blob.size <= targetAvatarUploadBytes) {
        const fileName = file.name.replace(/\.[^.]+$/, '') || 'avatar'
        return new File([blob], `${fileName}.${blob.type === 'image/webp' ? 'webp' : 'jpg'}`, { type: blob.type })
      }
    }
  }

  throw new Error(t('profile.avatar.compressTooLarge'))
}

async function prepareAvatarUpload(file: File): Promise<File> {
  if (!file.type.startsWith('image/')) {
    throw new Error(t('profile.avatar.invalidType'))
  }
  if (file.type === 'image/gif') {
    if (file.size > targetAvatarUploadBytes) {
      throw new Error(t('profile.avatar.gifTooLarge'))
    }
    return file
  }
  if (file.size <= targetAvatarUploadBytes) {
    return file
  }
  return compressAvatarFile(file)
}

async function handleAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (input) input.value = ''
  if (!file || avatarSaving.value || !props.user) return
  const id = props.user.id
  const seq = ++operation
  const current = () => !disposed && operation === seq && props.user?.id === id && authStore.user?.id === id
  avatarSaving.value = true
  try {
    const prepared = await prepareAvatarUpload(file)
    const normalized = normalizeUploadedAvatar(await readFileAsDataURL(prepared))
    if (!normalized || !current()) return
    avatarDraft.value = normalized
    const updated = await userAPI.updateProfile({ avatar_url: normalized })
    if (!current()) return
    savedAvatar.value = updated.avatar_url?.trim() || ''
    // Do not replace a concurrently refreshed balance or other profile fields.
    authStore.user = { ...authStore.user!, avatar_url: updated.avatar_url }
    appStore.showSuccess(t('profile.avatar.saveSuccess'))
  } catch (error: unknown) {
    if (current()) appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    if (current()) { avatarDraft.value = ''; avatarSaving.value = false }
  }
}

async function handleAvatarDelete() {
  if (avatarSaving.value || !props.user) return
  if (!savedAvatar.value) { appStore.showError(t('profile.avatar.emptyDeleteHint')); return }
  const id = props.user.id
  const seq = ++operation
  const current = () => !disposed && operation === seq && props.user?.id === id && authStore.user?.id === id
  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: '' })
    if (!current()) return
    authStore.user = { ...authStore.user!, avatar_url: updated.avatar_url }
    savedAvatar.value = ''
    avatarDraft.value = ''
    appStore.showSuccess(t('profile.avatar.deleteSuccess'))
  } catch (error: unknown) {
    if (current()) appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    if (current()) avatarSaving.value = false
  }
}
</script>
