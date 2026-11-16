/**
 * Where to go after signing in. Only same-site absolute paths are accepted, so a crafted
 * ?redirect= can never send the user to another origin or a javascript: URL.
 */
export function sanitizeRedirectPath(path: unknown, fallback = '/dashboard'): string {
  if (typeof path !== 'string' || !path) return fallback
  if (!path.startsWith('/')) return fallback
  if (path.startsWith('//') || path.startsWith('/\\')) return fallback
  if (path.includes('://')) return fallback
  if (path.includes('\n') || path.includes('\r')) return fallback
  return path
}

/** The ?redirect= to carry between the login and register pages, or undefined when there is none. */
export function redirectQuery(path: unknown): { redirect: string } | undefined {
  const target = sanitizeRedirectPath(path, '')
  return target ? { redirect: target } : undefined
}
