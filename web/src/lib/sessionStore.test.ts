import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import * as api from './api'
import { useSessionStore } from './sessionStore'

describe('sessionStore', () => {
  beforeEach(() => {
    // Reset the singleton store between tests.
    useSessionStore.setState({ sessions: [], activeId: '', loading: false, error: '' })
    vi.spyOn(api, 'listSessions').mockResolvedValue([
      { id: 's1', title: 'A', turns: 1, updated_at: 2, created_at: 1 },
    ])
    vi.spyOn(api, 'createSession').mockResolvedValue({ id: 's2', title: 'New', turns: 0, updated_at: 5, created_at: 5 })
    vi.spyOn(api, 'renameSession').mockResolvedValue()
    vi.spyOn(api, 'deleteSession').mockResolvedValue()
  })
  afterEach(() => vi.restoreAllMocks())

  it('load() populates sessions from the gateway', async () => {
    await useSessionStore.getState().load()
    expect(useSessionStore.getState().sessions.map((s) => s.id)).toEqual(['s1'])
  })

  it('create() prepends the new session and makes it active', async () => {
    await useSessionStore.getState().load()
    await useSessionStore.getState().create('/repo')
    expect(useSessionStore.getState().sessions[0].id).toBe('s2')
    expect(useSessionStore.getState().activeId).toBe('s2')
  })

  it('rename() updates the title in place', async () => {
    await useSessionStore.getState().load()
    await useSessionStore.getState().rename('s1', 'Renamed')
    expect(useSessionStore.getState().sessions.find((s) => s.id === 's1')!.title).toBe('Renamed')
    expect(api.renameSession).toHaveBeenCalledWith('s1', 'Renamed')
  })

  it('remove() drops the session and clears activeId when it was active', async () => {
    await useSessionStore.getState().load()
    useSessionStore.getState().setActive('s1')
    await useSessionStore.getState().remove('s1')
    expect(useSessionStore.getState().sessions.find((s) => s.id === 's1')).toBeUndefined()
    expect(useSessionStore.getState().activeId).toBe('')
    expect(api.deleteSession).toHaveBeenCalledWith('s1')
  })
})
