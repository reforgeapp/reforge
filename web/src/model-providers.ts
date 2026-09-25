export type ModelProviderOption = {
  id: string
  label: string
  provider: string
  profile: string
  endpoint: string
  endpointEditable: boolean
  secretRequired: boolean
}

export const modelProviderOptions: ModelProviderOption[] = [
  { id: 'openai', label: 'OpenAI', provider: 'openai', profile: 'responses', endpoint: 'https://api.openai.com/v1', endpointEditable: false, secretRequired: true },
  { id: 'anthropic', label: 'Claude (Anthropic)', provider: 'anthropic', profile: 'messages', endpoint: 'https://api.anthropic.com', endpointEditable: false, secretRequired: true },
  { id: 'google', label: 'Gemini (Google)', provider: 'google', profile: 'gemini', endpoint: 'https://generativelanguage.googleapis.com', endpointEditable: false, secretRequired: true },
  { id: 'opencode_zen', label: 'OpenCode Zen', provider: 'compatible', profile: 'opencode_zen', endpoint: 'https://opencode.ai/zen/v1', endpointEditable: false, secretRequired: true },
  { id: 'opencode_go', label: 'OpenCode Go', provider: 'compatible', profile: 'opencode_go', endpoint: 'https://opencode.ai/zen/go/v1', endpointEditable: false, secretRequired: true },
  { id: 'compatible', label: 'Custom / compatible', provider: 'compatible', profile: 'chat_completions', endpoint: '', endpointEditable: true, secretRequired: false },
]

export const compatibleProfiles = [
  { value: 'chat_completions', label: 'Chat Completions' },
  { value: 'responses', label: 'Responses' },
  { value: 'ollama', label: 'Ollama' },
  { value: 'vllm', label: 'vLLM' },
]

export const defaultModelProvider = modelProviderOptions[0]

export function modelProviderOption(id: string): ModelProviderOption {
  return modelProviderOptions.find(item => item.id === id) ?? defaultModelProvider
}

const providerLabels: Record<string, string> = {
  openai: 'OpenAI',
  anthropic: 'Claude (Anthropic)',
  google: 'Gemini (Google)',
  compatible: 'Compatible',
}

export function connectionProviderLabel(provider: string, profile?: string) {
  if (provider === 'compatible' && (profile === 'opencode_zen' || profile === 'opencode_go')) return 'OpenCode'
  return providerLabels[provider] ?? provider
}

export function modelProfileLabel(provider: string, profile?: string) {
  if (provider === 'compatible' && profile === 'opencode_zen') return 'OpenCode Zen'
  if (provider === 'compatible' && profile === 'opencode_go') return 'OpenCode Go'
  return profile ?? 'Unknown'
}

export type ConnectionStatusTone = 'green' | 'amber' | 'red' | 'neutral' | 'teal'

export type ConnectionStatus = {
  label: string
  tone: ConnectionStatusTone
  title?: string
}

export function connectionStateStatus(provider: string, profile: string | undefined, state: string): ConnectionStatus {
  if (state === 'healthy' && provider === 'compatible' && (profile === 'opencode_zen' || profile === 'opencode_go')) {
    return {
      label: 'Key untested',
      tone: 'amber',
      title: 'Model found in the provider catalogue. The API key is checked on first use.',
    }
  }
  return {
    label: state,
    tone: state === 'healthy' ? 'green' : state === 'revoked' ? 'red' : 'amber',
  }
}
