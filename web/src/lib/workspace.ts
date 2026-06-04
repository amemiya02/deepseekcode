// Workspace API — the SINGLE source of FileEntry + the file-tree/changed-file
// readers (Contract 1, C-3). CREATED here in Wave 0 (minimal); Wave 5 EXTENDS
// this module (it must NOT recreate it or redefine FileEntry/fetchFiles). The
// data comes from Wave 1's /v1/files, /v1/file, /v1/changed and Wave 5's
// /v1/add-to-chat. Framework-agnostic plain TS (no React) so components AND
// plain modules can import it.

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
}

export interface FileContent {
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

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return (await res.json()) as T
}

// fetchFiles lists one directory level under the workspace root (path "" = root).
export async function fetchFiles(path?: string): Promise<FileEntry[]> {
  const url = path ? `/v1/files?path=${encodeURIComponent(path)}` : '/v1/files'
  const data = await getJSON<{ entries: FileEntry[] }>(url)
  return data.entries
}

// fetchFile reads one file's content + binary/truncation flags.
export async function fetchFile(path: string): Promise<FileContent> {
  return getJSON<FileContent>(`/v1/file?path=${encodeURIComponent(path)}`)
}

// fetchChanged returns the workspace's changed files (git porcelain).
export async function fetchChanged(): Promise<ChangedEntry[]> {
  const data = await getJSON<{ entries: ChangedEntry[] }>('/v1/changed')
  return data.entries
}

// addToChat asks the gateway to attach a file to the current chat context.
export async function addToChat(path: string): Promise<void> {
  const res = await fetch('/v1/add-to-chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
}
