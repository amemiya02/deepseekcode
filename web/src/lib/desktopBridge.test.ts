import { afterEach, describe, expect, it, vi } from 'vitest'
import { getPort, getVersion, isDesktop, openDownload, openFileDialog } from './desktopBridge'

// Helper to mutate the global runtime shape the bridge feature-detects.
type Win = {
  _wails?: unknown
  wails?: unknown
  runtime?: unknown
}
function w(): Win {
  return window as unknown as Win
}

afterEach(() => {
  delete w()._wails
  delete w().wails
  delete w().runtime
  vi.restoreAllMocks()
})

describe('desktopBridge', () => {
  describe('isDesktop', () => {
    it('is false in a plain browser', () => {
      expect(isDesktop()).toBe(false)
    })
    it('is true when the v3 _wails marker is present', () => {
      w()._wails = {}
      expect(isDesktop()).toBe(true)
    })
    it('is true when a wails runtime object is present', () => {
      w().wails = {}
      expect(isDesktop()).toBe(true)
    })
  })

  describe('bound method calls (v3 Call.ByName)', () => {
    it('getVersion routes through App.GetVersion', async () => {
      const byName = vi.fn().mockResolvedValue('v1.2.3')
      w().wails = { Call: { ByName: byName } }
      await expect(getVersion()).resolves.toBe('v1.2.3')
      expect(byName).toHaveBeenCalledWith('App.GetVersion')
    })

    it('getPort routes through App.GetPort', async () => {
      const byName = vi.fn().mockResolvedValue(7432)
      w().wails = { Call: { ByName: byName } }
      await expect(getPort()).resolves.toBe(7432)
      expect(byName).toHaveBeenCalledWith('App.GetPort')
    })

    it('openFileDialog returns the selected path', async () => {
      const byName = vi.fn().mockResolvedValue('/tmp/x.go')
      w().wails = { Call: { ByName: byName } }
      await expect(openFileDialog()).resolves.toBe('/tmp/x.go')
      expect(byName).toHaveBeenCalledWith('App.OpenFileDialog')
    })

    it('openFileDialog returns undefined when cancelled (empty path)', async () => {
      w().wails = { Call: { ByName: vi.fn().mockResolvedValue('') } }
      await expect(openFileDialog()).resolves.toBeUndefined()
    })

    it('returns undefined in a plain browser (no runtime)', async () => {
      await expect(getVersion()).resolves.toBeUndefined()
      await expect(getPort()).resolves.toBeUndefined()
      await expect(openFileDialog()).resolves.toBeUndefined()
    })

    it('swallows runtime call errors and returns undefined', async () => {
      w().wails = { Call: { ByName: vi.fn().mockRejectedValue(new Error('boom')) } }
      await expect(getVersion()).resolves.toBeUndefined()
    })
  })

  describe('openDownload', () => {
    it('uses the v3 Browser.OpenURL when present', () => {
      const open = vi.fn().mockResolvedValue(undefined)
      w().wails = { Browser: { OpenURL: open } }
      openDownload('https://dl/v2')
      expect(open).toHaveBeenCalledWith('https://dl/v2')
    })

    it('falls back to the v2 runtime.BrowserOpenURL shim', () => {
      const open = vi.fn()
      w().runtime = { BrowserOpenURL: open }
      openDownload('https://dl/v2')
      expect(open).toHaveBeenCalledWith('https://dl/v2')
    })

    it('falls back to window.open in a plain browser', () => {
      const open = vi.spyOn(window, 'open').mockReturnValue(null)
      openDownload('https://dl/v2')
      expect(open).toHaveBeenCalledWith('https://dl/v2', '_blank', 'noopener')
    })
  })
})
