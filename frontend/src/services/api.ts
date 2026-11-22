export interface ProblemErrorDetail {
  message?: string
  location?: string
  value?: unknown
  code?: string
  detected?: string[]
}

export class ApiError extends Error {
  status: number
  detail: string
  errors?: ProblemErrorDetail[]

  constructor(status: number, detail: string, errors?: ProblemErrorDetail[]) {
    super(detail || `HTTP ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.detail = detail
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
    throw new ApiError(resp.status, p.detail || p.title || `Request failed (${resp.status})`, p.errors)
  }

  return parsed as T
}
