import { resolveKittySite } from './siteIdentity'

export const kittySite = resolveKittySite(import.meta.env.VITE_KITTY_SITE)
