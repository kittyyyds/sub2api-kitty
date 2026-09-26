/**
 * Type definitions for Vue Router meta fields
 * Extends the RouteMeta interface with custom properties
 */

import 'vue-router'

declare module 'vue-router' {
  interface RouteMeta {
    /**
     * Whether this route requires authentication
     * @default true
     */
    requiresAuth?: boolean

    /**
     * Whether this route requires admin role
     * @default false
     */
    requiresAdmin?: boolean
<<<<<<< 6e3949df120b57ee87b7d1362596a0a40d242e40
    requiresSuperAdmin?: boolean
=======
    requiresAccountManagement?: boolean
>>>>>>> 4f084496971c8a40d28936b6e9dc955137b75c66
    requiresRequestCapture?: boolean

    /**
     * Page title for this route
     */
    title?: string

    /**
     * Optional breadcrumb items for navigation
     */
    breadcrumbs?: Array<{
      label: string
      to?: string
    }>

    /**
     * Icon name for this route (for sidebar navigation)
     */
    icon?: string

    /**
     * Whether to hide this route from navigation menu
     * @default false
     */
    hideInMenu?: boolean

    /**
     * Whether this route requires internal payment system to be enabled
     * @default false
     */
    requiresPayment?: boolean

    /**
     * Whether this route requires the channel monitor feature to be enabled
     * @default false
     */
    requiresChannelMonitor?: boolean

    /**
     * 是否要求订阅功能开关（subscription_enabled，opt-out）未被显式关闭
     * @default false
     */
    requiresSubscription?: boolean

    /**
     * i18n key for the page title
     */
    titleKey?: string

    /**
     * i18n key for the page description
     */
    descriptionKey?: string
  }
}
