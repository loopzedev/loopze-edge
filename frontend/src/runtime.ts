// Runtime configuration injected by the Go backend at index.html render
// time. Lets a single build be served at "/" or under any subpath
// without rebuilding the frontend. The backend stamps a <base href>
// into the document and sets window.__LOOPZE_BASE__ to the matching
// value (always with a trailing slash, e.g. "/" or "/loopze/").

declare global {
  interface Window {
    __LOOPZE_BASE__?: string
  }
}

function readBase(): string {
  const injected = typeof window !== 'undefined' ? window.__LOOPZE_BASE__ : ''
  if (injected && injected.length > 0) return injected
  // Fallback: derive from <base href> if present, else assume root.
  if (typeof document !== 'undefined' && document.baseURI) {
    try {
      return new URL(document.baseURI).pathname
    } catch {
      /* ignore */
    }
  }
  return '/'
}

/** Always with a trailing slash, e.g. "/" or "/loopze/". */
export const basePath: string = readBase()

/** Without trailing slash; used to build absolute paths like
 *  `${basePathNoSlash}/api/v1/foo`. Empty string when at root. */
export const basePathNoSlash: string =
  basePath === '/' ? '' : basePath.replace(/\/$/, '')

/** Read the value of a cookie by name. Returns "" when absent. */
export function readCookie(name: string): string {
  if (typeof document === 'undefined') return ''
  const prefix = name + '='
  for (const part of document.cookie.split(';')) {
    const trimmed = part.trim()
    if (trimmed.startsWith(prefix)) {
      return decodeURIComponent(trimmed.slice(prefix.length))
    }
  }
  return ''
}
