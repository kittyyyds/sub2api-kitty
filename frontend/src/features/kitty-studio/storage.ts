import type { StudioForm, StudioResult } from './types'

const DATABASE_NAME = 'kitty-studio-v1'
const STORE_NAME = 'results'
const DRAFT_KEY = 'kitty-studio:draft:v1'
const MAX_RESULTS = 80

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE_NAME, 1)
    request.onerror = () => reject(request.error || new Error('本地画廊数据库打开失败。'))
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(STORE_NAME)) {
        const store = database.createObjectStore(STORE_NAME, { keyPath: 'id' })
        store.createIndex('createdAt', 'createdAt')
      }
    }
    request.onsuccess = () => resolve(request.result)
  })
}

async function withStore<T>(
  mode: IDBTransactionMode,
  work: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
  const database = await openDatabase()
  return new Promise((resolve, reject) => {
    const transaction = database.transaction(STORE_NAME, mode)
    const request = work(transaction.objectStore(STORE_NAME))
    request.onerror = () => reject(request.error || new Error('本地画廊操作失败。'))
    request.onsuccess = () => resolve(request.result)
    transaction.oncomplete = () => database.close()
    transaction.onerror = () => {
      database.close()
      reject(transaction.error || new Error('本地画廊事务失败。'))
    }
  })
}

export async function listStudioResults(): Promise<StudioResult[]> {
  if (typeof indexedDB === 'undefined') return []
  const values = await withStore<StudioResult[]>('readonly', (store) => store.getAll())
  return values.sort((a, b) => b.createdAt - a.createdAt).slice(0, MAX_RESULTS)
}

export async function saveStudioResult(result: StudioResult): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  await withStore<IDBValidKey>('readwrite', (store) => store.put(result))
  const all = await withStore<StudioResult[]>('readonly', (store) => store.getAll())
  all.sort((a, b) => b.createdAt - a.createdAt)
  await Promise.all(all.slice(MAX_RESULTS).map((item) => deleteStudioResult(item.id)))
}

export async function deleteStudioResult(id: string): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  await withStore<undefined>('readwrite', (store) => store.delete(id))
}

export async function clearStudioResults(): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  await withStore<undefined>('readwrite', (store) => store.clear())
}

export function loadStudioDraft(fallback: StudioForm): StudioForm {
  try {
    const parsed = JSON.parse(localStorage.getItem(DRAFT_KEY) || '{}') as Partial<StudioForm>
    return { ...fallback, ...parsed, prompt: typeof parsed.prompt === 'string' ? parsed.prompt.slice(0, 8000) : fallback.prompt }
  } catch {
    return { ...fallback }
  }
}

export function saveStudioDraft(form: StudioForm): void {
  localStorage.setItem(DRAFT_KEY, JSON.stringify(form))
}
