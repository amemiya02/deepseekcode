// Typed client for the Wave-6 system/settings endpoints. Sibling of api.ts:
// api.ts owns the single streaming GatewayClient (Contract 1); these are the
// plain REST reads/writes the Settings/Onboarding/Update surfaces use. Field
// names mirror the Go DTOs in internal/gateway/{config,onboarding,doctor,update}.go
// — keep them in lockstep.

export interface ConfigDTO {
  theme: string
  accent: string
  density: string
  language: string
  transcriptVerbosity: 'normal' | 'verbose' | 'summary'
  model: string
  reasoningEffort: string
  baseUrl: string
  autoRoute: boolean
  escalationModel: string
  duetEnabled: boolean
  sandboxEnabled: boolean
  sandboxNetwork: boolean
  autoReasoning: boolean
  autoClarify: boolean
  // Network / proxy
  proxyMode: 'auto' | 'env' | 'custom' | 'off'
  proxyScheme: 'http' | 'https' | 'socks5' | 'socks5h'
  proxyUrl: string
  noProxy: string
  // Keybindings
  vimKeybindings: boolean
  // Budget / tools
  maxReadBytes: number
  maxWriteBytes: number
  // Permissions
  permissionDefault: string
  // Sessions & storage
  sessionTTLDays: number
  sessionSnapshotKeep: number
  sessionAutoResumeAge: number
  // Editor / appearance
  transparentBackground: boolean
}

export interface DoctorCheck {
  name: string
  ok: boolean
  detail: string
}
export interface DoctorReport {
  allOk: boolean
  checks: DoctorCheck[]
}

export interface UpdateInfo {
  current: string
  latest: string
  updateAvailable: boolean
  url: string
}

export interface OnboardingStatus {
  needsOnboarding: boolean
  baseUrl: string
  model: string
}

export interface ConnectKeyInput {
  apiKey: string
  baseUrl: string
  model: string
}

async function jsonOrThrow<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `gateway error ${res.status}`)
  }
  return res.json() as Promise<T>
}

export async function fetchConfig(): Promise<ConfigDTO> {
  return jsonOrThrow<ConfigDTO>(await fetch('/v1/config'))
}

export async function saveConfig(patch: Partial<ConfigDTO>): Promise<ConfigDTO> {
  return jsonOrThrow<ConfigDTO>(
    await fetch('/v1/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  )
}

export async function fetchDoctor(): Promise<DoctorReport> {
  return jsonOrThrow<DoctorReport>(await fetch('/v1/doctor'))
}

export async function fetchUpdate(): Promise<UpdateInfo> {
  return jsonOrThrow<UpdateInfo>(await fetch('/v1/update'))
}

export async function fetchOnboarding(): Promise<OnboardingStatus> {
  return jsonOrThrow<OnboardingStatus>(await fetch('/v1/onboarding'))
}

export async function connectKey(input: ConnectKeyInput): Promise<void> {
  const res = await fetch('/v1/connect-key', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `gateway error ${res.status}`)
  }
}

export interface McpServer {
  id: string
  name: string
  enabled: boolean
  transport: string
  command: string
  status?: string
  toolCount?: number
}

export interface McpUpsert {
  name: string
  transport: string
  command?: string
  url?: string
  args?: string[]
  env?: Record<string, string>
  disabled?: boolean
  timeoutSeconds?: number
}

export async function fetchMcpServers(): Promise<McpServer[]> {
  const body = await jsonOrThrow<{ items: McpServer[] }>(await fetch('/v1/mcp'))
  return body.items ?? []
}

export async function saveMcpServer(u: McpUpsert): Promise<McpServer[]> {
  const body = await jsonOrThrow<{ items: McpServer[] }>(
    await fetch('/v1/mcp', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(u) }),
  )
  return body.items ?? []
}

export async function deleteMcpServer(name: string): Promise<McpServer[]> {
  const body = await jsonOrThrow<{ items: McpServer[] }>(
    await fetch(`/v1/mcp/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  )
  return body.items ?? []
}

export async function toggleMcpServer(name: string, disabled: boolean): Promise<McpServer[]> {
  const body = await jsonOrThrow<{ items: McpServer[] }>(
    await fetch(`/v1/mcp/${encodeURIComponent(name)}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ disabled }),
    }),
  )
  return body.items ?? []
}
