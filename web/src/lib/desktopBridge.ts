// desktopBridge — a thin, shell-agnostic adapter between the SPA and the Wails
// desktop runtime.
//
// The SPA renders identically in a plain browser (`dsc serve`) and inside the
// Wails v3 desktop shell. This module isolates every desktop-only capability
// behind feature-detected functions so no component needs to know which shell
// it is running in (the spec's "thin lib/desktopBridge.ts adapter").
//
// Wails v3 changes the runtime surface from v2:
//   - bound Go methods are invoked via the runtime Call.ByName("App.Method", …)
//     (v2 exposed them as window.go.App.* — that path no longer exists);
//   - URLs are opened via Browser.OpenURL (v2 used window.runtime.BrowserOpenURL).
//
// To keep the SPA free of a hard build dependency on @wailsio/runtime (so the
// browser build and unit tests stay green), the bridge reaches the runtime
// dynamically off the global object and falls back gracefully when absent.

// --- Runtime shapes (structural; we never import the wails package) ----------

interface WailsCalls {
  // v3: Call.ByName("App.GetVersion", ...args) => Promise<result>
  ByName?: (method: string, ...args: unknown[]) => Promise<unknown>
}

interface WailsBrowser {
  // v3: Browser.OpenURL(url) => Promise<void>
  OpenURL?: (url: string | URL) => Promise<void>
}

interface WailsRuntime {
  Call?: WailsCalls
  Browser?: WailsBrowser
  // v2 compatibility: some builds expose BrowserOpenURL directly on runtime.
  BrowserOpenURL?: (url: string) => void
}

interface DesktopGlobals {
  // v3 sets window._wails as its runtime marker.
  _wails?: unknown
  // The shared runtime object (v3 "@wailsio/runtime" attaches helpers here in
  // some embeddings; v2 attached window.runtime).
  wails?: WailsRuntime
  runtime?: WailsRuntime
}

function globals(): DesktopGlobals {
  return window as unknown as DesktopGlobals
}

/**
 * isDesktop reports whether the SPA is running inside the Wails desktop shell
 * (as opposed to a plain browser served by `dsc serve`).
 */
export function isDesktop(): boolean {
  const g = globals()
  return typeof g._wails !== 'undefined' || typeof g.wails !== 'undefined'
}

function runtime(): WailsRuntime | undefined {
  const g = globals()
  return g.wails ?? g.runtime
}

/**
 * call invokes a bound Go method by its qualified name ("App.GetVersion") via
 * the Wails v3 runtime. Returns undefined when not running under the desktop
 * shell or when the runtime/method is unavailable, so callers can fall back.
 */
async function call<T>(method: string, ...args: unknown[]): Promise<T | undefined> {
  const byName = runtime()?.Call?.ByName
  if (typeof byName !== 'function') return undefined
  try {
    return (await byName(method, ...args)) as T
  } catch {
    return undefined
  }
}

// --- Public adapter ----------------------------------------------------------

/**
 * getVersion returns the dsc binary version string from the desktop shell, or
 * undefined in a plain browser (where the caller uses the gateway/REST source).
 */
export async function getVersion(): Promise<string | undefined> {
  return call<string>('App.GetVersion')
}

/**
 * getPort returns the in-process gateway port reported by the desktop shell, or
 * undefined in a plain browser (where the Vite proxy / same-origin host is used).
 */
export async function getPort(): Promise<number | undefined> {
  return call<number>('App.GetPort')
}

/**
 * openFileDialog opens the native OS file picker (desktop only) and resolves to
 * the selected path, or undefined when cancelled or when not in the desktop
 * shell.
 */
export async function openFileDialog(): Promise<string | undefined> {
  const path = await call<string>('App.OpenFileDialog')
  return path && path.length > 0 ? path : undefined
}

/**
 * openDownload opens a URL in the user's default browser. In the desktop shell
 * it uses the Wails runtime (Browser.OpenURL, or the v2 BrowserOpenURL shim);
 * in a plain browser it falls back to window.open.
 */
export function openDownload(url: string): void {
  const rt = runtime()
  const openURL = rt?.Browser?.OpenURL
  if (typeof openURL === 'function') {
    void openURL(url)
    return
  }
  // v2-style runtime shim (kept so the existing UpdateBanner contract holds).
  const legacy = rt?.BrowserOpenURL
  if (typeof legacy === 'function') {
    legacy(url)
    return
  }
  window.open(url, '_blank', 'noopener')
}
