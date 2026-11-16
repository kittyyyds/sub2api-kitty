import { readonly, shallowRef } from 'vue'

export type AppTheme = 'light' | 'dark'

const THEME_STORAGE_KEY = 'theme'
const isDark = shallowRef(false)
let initialized = false

export function resolveInitialTheme(storage: Pick<Storage, 'getItem'>): AppTheme {
  return storage.getItem(THEME_STORAGE_KEY) === 'dark' ? 'dark' : 'light'
}

function applyTheme(theme: AppTheme): void {
  isDark.value = theme === 'dark'
  document.documentElement.classList.toggle('dark', isDark.value)
  document.documentElement.style.colorScheme = theme
}

function handleStorageChange(event: StorageEvent): void {
  if (event.key !== THEME_STORAGE_KEY) return
  applyTheme(event.newValue === 'dark' ? 'dark' : 'light')
}

export function initializeTheme(): AppTheme {
  const theme = resolveInitialTheme(window.localStorage)
  applyTheme(theme)
  if (!initialized) {
    window.addEventListener('storage', handleStorageChange)
    initialized = true
  }
  return theme
}

export function setTheme(theme: AppTheme): void {
  applyTheme(theme)
  window.localStorage.setItem(THEME_STORAGE_KEY, theme)
}

export function useTheme() {
  if (!initialized) initializeTheme()

  function toggleTheme(): void {
    setTheme(isDark.value ? 'light' : 'dark')
  }

  return {
    isDark: readonly(isDark),
    setTheme,
    toggleTheme,
  }
}
