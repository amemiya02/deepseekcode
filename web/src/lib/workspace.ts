// Typed client for the Wave-5 workspace surface (spec §9, Contract 1): file
// tree, file read, git changed files, and add-to-chat. Reads are GET (served by
// Wave 1); add-to-chat is POST (Wave 5). Framework-agnostic: no React imports.

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
}

export interface FilesResult {
  entries: FileEntry[]
}

export interface FileResult {
  path: string
  content: string
  binary: boolean
  truncated: boolean
}

export interface ChangedEntry {
  path: string
  status: string
  deleted: boolean
}

export interface ChangedResult {
  entries: ChangedEntry[]
}

export interface AddToChatRequest {
  path?: string
  text?: string
  is_dir?: boolean
  include_contents?: boolean
}

export interface AddToChatResult {
  label: string
  content: string
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<T>
}

export function fetchFiles(path = ''): Promise<FilesResult> {
  return getJSON<FilesResult>(`/v1/files?path=${encodeURIComponent(path)}`)
}

export function fetchFile(path: string): Promise<FileResult> {
  return getJSON<FileResult>(`/v1/file?path=${encodeURIComponent(path)}`)
}

export function fetchChanged(): Promise<ChangedResult> {
  return getJSON<ChangedResult>('/v1/changed')
}

export async function addToChat(req: AddToChatRequest): Promise<AddToChatResult> {
  const res = await fetch('/v1/add-to-chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<AddToChatResult>
}
