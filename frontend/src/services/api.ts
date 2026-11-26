export interface ProblemErrorDetail {
  message?: string
  location?: string
  value?: unknown
  code?: string
  detected?: string[]
}

function humanizeFieldLocation(location?: string): string {
  if (!location) return ''
  const cleaned = location
    .replace(/^(body|query|path|header)\./i, '')
    .replace(/\[(\d+)\]/g, ' #$1')
    .split('.')
    .filter(Boolean)
    .map((part) =>
      part
        .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
        .replace(/_/g, ' ')
        .trim(),
    )
    .join(' ')
  if (!cleaned) return ''
  return cleaned.charAt(0).toUpperCase() + cleaned.slice(1)
}

function cleanRawBackendError(raw: string): string {
  let msg = raw.trim()
  if (!msg) return ''
  // Strip raw PostgreSQL SQLSTATE suffixes and "ERROR: " prefixes
  msg = msg.replace(/^ERROR:\s*/i, '').replace(/\s*\(SQLSTATE\s+[0-9A-Z]+\)\s*$/i, '')
  if (/invalid input syntax for type inet/i.test(msg)) {
    return 'Invalid IP address format.'
  }
  if (/duplicate key value violates unique constraint/i.test(msg)) {
    return 'An item with this name or identifier already exists.'
  }
  if (/violates not-null constraint/i.test(msg)) {
    return 'A required field is missing.'
  }
  if (/violates check constraint/i.test(msg)) {
    return 'One or more values are outside the allowed range.'
  }
  return msg
}

export function formatProblemDetails(detail?: string, title?: string, errors?: ProblemErrorDetail[], status?: number): string {
  const details: string[] = []
  if (Array.isArray(errors) && errors.length > 0) {
    for (const item of errors) {
      const m = (item.message || '').trim()
      if (!m) continue
      const field = humanizeFieldLocation(item.location)
      details.push(field ? `${field}: ${m}` : m)
    }
  }
  if (details.length > 0) {
    return details.join(' | ')
  }
  const primary = cleanRawBackendError(detail || title || '')
  if (primary) return primary
  if (status) {
    if (status === 401) return 'Your session expired. Please sign in again.'
    if (status === 403) return 'You do not have permission to perform this action.'
    if (status === 404) return 'The requested resource was not found.'
    if (status === 409) return 'A conflicting record already exists.'
    if (status >= 500) return `Server error (HTTP ${status}). Please try again.`
    return `Request failed (HTTP ${status}).`
  }
  return 'Request failed.'
}

export function formatErrorMessage(err: unknown, fallback = 'An unexpected error occurred.'): string {
  if (err instanceof ApiError) {
    return err.message || fallback
  }
  if (err instanceof TypeError && /fetch|network/i.test(err.message)) {
    return 'Unable to reach the server. Check your network connection and try again.'
  }
  if (err instanceof Error) {
    const cleaned = cleanRawBackendError(err.message)
    return cleaned || fallback
  }
  if (typeof err === 'string' && err.trim()) {
    return cleanRawBackendError(err)
  }
  return fallback
}

export class ApiError extends Error {
  status: number
  detail: string
  errors?: ProblemErrorDetail[]

  constructor(status: number, detail: string, errors?: ProblemErrorDetail[], title?: string) {
    const formatted = formatProblemDetails(detail, title, errors, status)
    super(formatted)
    this.name = 'ApiError'
    this.status = status
    this.detail = formatted
    this.errors = errors
  }

  isCatchAllAllowedIPs(): boolean {
    if (!this.errors) return false
    return this.errors.some(
      (e) => e.code === 'CATCHALL_ALLOWED_IPS' || e.message === 'CATCHALL_ALLOWED_IPS',
    )
  }
}

type TokenGetter = () => string | null
type RefreshFn = () => Promise<string | null>

let getAccessToken: TokenGetter = () => null
let refreshAccessToken: RefreshFn = async () => null
let inflightRefresh: Promise<string | null> | null = null

export function configureApiAuth(getter: TokenGetter, refresher: RefreshFn) {
  getAccessToken = getter
  refreshAccessToken = refresher
}

async function doRefreshOnce(): Promise<string | null> {
  if (!inflightRefresh) {
    inflightRefresh = refreshAccessToken().finally(() => {
      inflightRefresh = null
    })
  }
  return inflightRefresh
}

export async function apiRequest<T>(
  path: string,
  options: RequestInit & { skipAuth?: boolean; _retried?: boolean } = {},
): Promise<T> {
  const headers = new Headers(options.headers || {})
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (!options.skipAuth) {
    const token = getAccessToken()
    if (token) {
      headers.set('Authorization', `Bearer ${token}`)
    }
  }

  const resp = await fetch(path, {
    ...options,
    headers,
    credentials: 'include', // sends HttpOnly refresh cookie automatically on /auth/*
  })

  if (resp.status === 401 && !options.skipAuth && !options._retried) {
    const fresh = await doRefreshOnce()
    if (fresh) {
      return apiRequest<T>(path, { ...options, _retried: true })
    }
  }

  if (resp.status === 204 || resp.status === 202) {
    return undefined as T
  }

  const text = await resp.text()
  let parsed: unknown = null
  if (text) {
    try {
      parsed = JSON.parse(text)
    } catch {
      parsed = { detail: text }
    }
  }

  if (!resp.ok) {
    const p = (parsed || {}) as { detail?: string; title?: string; errors?: ProblemErrorDetail[] }
    throw new ApiError(resp.status, p.detail || '', p.errors, p.title)
  }

  return parsed as T
}
