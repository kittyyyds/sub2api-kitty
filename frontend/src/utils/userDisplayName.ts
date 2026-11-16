export function userDisplayName(user?: { username?: string | null; email?: string | null } | null): string {
  return user?.username?.trim() || user?.email?.split('@')[0]?.trim() || ''
}
