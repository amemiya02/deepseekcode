import { create } from 'zustand'
import { listSessions, createSession, renameSession, deleteSession, type Session } from './api'

interface SessionState {
  sessions: Session[]
  activeId: string
  loading: boolean
  error: string
  load: () => Promise<void>
  create: (workingDir: string) => Promise<Session>
  rename: (id: string, title: string) => Promise<void>
  remove: (id: string) => Promise<void>
  setActive: (id: string) => void
}

// The single session store. Consumers select with useSessionStore(s => s.sessions);
// imperative tests use useSessionStore.getState()/.setState().
export const useSessionStore = create<SessionState>((set, get) => ({
  sessions: [],
  activeId: '',
  loading: true,
  error: '',

  async load() {
    set({ loading: true, error: '' })
    try {
      set({ sessions: await listSessions() })
    } catch (e) {
      set({ error: e instanceof Error ? e.message : String(e) })
    } finally {
      set({ loading: false })
    }
  },

  async create(workingDir) {
    const s = await createSession(workingDir)
    set({ sessions: [s, ...get().sessions], activeId: s.id })
    return s
  },

  async rename(id, title) {
    await renameSession(id, title)
    set({ sessions: get().sessions.map((s) => (s.id === id ? { ...s, title } : s)) })
  },

  async remove(id) {
    await deleteSession(id)
    set({
      sessions: get().sessions.filter((s) => s.id !== id),
      activeId: get().activeId === id ? '' : get().activeId,
    })
  },

  setActive(id) {
    set({ activeId: id })
  },
}))
