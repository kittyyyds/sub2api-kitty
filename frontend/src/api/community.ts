import { apiClient } from './client'

export interface CommunityBenefit {
  id: string
  title: string
  highlight: string
  lines: string[]
  command: string
}
export interface CommunityGuide {
  enabled: boolean
  chat_enabled: boolean
  auto_email_binding: boolean
  join_review?: { min_spent: number; question: string }
  benefits: CommunityBenefit[]
  commands: { command: string; description: string }[]
}

export async function getCommunityGuide(): Promise<CommunityGuide> {
  return (await apiClient.get<CommunityGuide>('/settings/community')).data
}
