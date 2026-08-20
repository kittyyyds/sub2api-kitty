export interface HeyRouteTool {
  name: string
  slug: string
  src: string
}

export const heyRouteTools: HeyRouteTool[] = [
  { name: 'Codex', slug: 'codex', src: '/images/heyroute/integrations/codex.png' },
  { name: 'Claude Code', slug: 'claude-code', src: '/images/heyroute/integrations/claude-code.svg' },
  { name: 'Cursor', slug: 'cursor', src: '/images/heyroute/integrations/cursor.svg' },
  { name: 'VS Code', slug: 'vscode', src: '/images/heyroute/integrations/vscode.svg' },
  { name: 'OpenClaw', slug: 'openclaw', src: '/images/heyroute/integrations/openclaw.svg' },
  { name: 'Hermes', slug: 'hermes', src: '/images/heyroute/integrations/hermes.png' },
  { name: 'WorkBuddy', slug: 'workbuddy', src: '/images/heyroute/integrations/workbuddy.svg' },
  { name: 'OpenAI', slug: 'openai', src: '/images/heyroute/integrations/openai.svg' },
]

export const primaryHeyRouteTools = heyRouteTools.slice(0, 6)
