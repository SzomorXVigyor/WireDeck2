const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/
const HEX_RE = /^#[0-9a-fA-F]{6}$/

export function isValidIPv4(input: string): boolean {
  const parts = input.split('.')
  if (parts.length !== 4) return false
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) return false
    if (part.length > 1 && part.startsWith('0')) return false
    const n = Number(part)
    if (n < 0 || n > 255) return false
  }
  return true
}

export function isValidIPv6(input: string): boolean {
  if (!input.includes(':') || input.includes(':::')) return false
  const halves = input.split('::')
  if (halves.length > 2) return false
  const left = halves[0] ? halves[0].split(':') : []
  const right = halves[1] ? halves[1].split(':') : []
  const isGroup = (g: string) => /^[0-9a-fA-F]{1,4}$/.test(g)
  if (!left.every(isGroup) || !right.every(isGroup)) return false
  const total = left.length + right.length
  return halves.length === 2 ? total < 8 : total === 8
}

export function isValidIpAddress(input: string): boolean {
  const trimmed = input.trim()
  if (!trimmed) return false
  return isValidIPv4(trimmed) || isValidIPv6(trimmed)
}

export function validateRequiredText(
  value: string,
  label: string,
  opts: { min?: number; max?: number } = {},
): string | null {
  const trimmed = value.trim()
  if (!trimmed) return `${label} is required.`
  if (opts.min !== undefined && trimmed.length < opts.min) {
    return `${label} must be at least ${opts.min} characters.`
  }
  if (opts.max !== undefined && trimmed.length > opts.max) {
    return `${label} must be ${opts.max} characters or fewer.`
  }
  return null
}

export function validateOptionalText(value: string, label: string, max: number): string | null {
  if (value.trim().length > max) {
    return `${label} must be ${max} characters or fewer.`
  }
  return null
}

export function validateIpAddress(value: string, label = 'IP address'): string | null {
  const trimmed = value.trim()
  if (!trimmed) return `${label} is required.`
  if (!isValidIpAddress(trimmed)) {
    return `${label} must be a valid IPv4 or IPv6 address (e.g. 192.168.1.10).`
  }
  return null
}

export function validatePort(value: string | number, label = 'Port'): string | null {
  return validateIntegerRange(value, label, 1, 65535, { required: true })
}

export function validateEmail(value: string, label = 'Email'): string | null {
  const trimmed = value.trim()
  if (!trimmed) return `${label} is required.`
  if (trimmed.length > 254 || !EMAIL_RE.test(trimmed)) {
    return `${label} must be a valid email address.`
  }
  return null
}

export function validateSlug(value: string, label = 'Slug'): string | null {
  const trimmed = value.trim()
  if (!trimmed) return `${label} is required.`
  if (!SLUG_RE.test(trimmed)) {
    return `${label} must use lowercase letters, numbers, and hyphens only.`
  }
  return null
}

export function validateIntegerRange(
  value: string | number,
  label: string,
  min: number,
  max: number,
  opts: { required?: boolean } = { required: true },
): string | null {
  const raw = String(value ?? '').trim()
  if (!raw) {
    return opts.required ? `${label} is required.` : null
  }
  if (!/^-?\d+$/.test(raw)) {
    return `${label} must be a whole number.`
  }
  const num = Number(raw)
  if (!Number.isSafeInteger(num) || num < min || num > max) {
    return `${label} must be between ${min} and ${max}.`
  }
  return null
}

export function validateNumberRange(
  value: string | number,
  label: string,
  min?: number,
  max?: number,
  opts: { required?: boolean } = { required: true },
): string | null {
  const raw = String(value ?? '').trim()
  if (!raw) {
    return opts.required ? `${label} is required.` : null
  }
  const num = Number(raw)
  if (!Number.isFinite(num)) {
    return `${label} must be a valid number.`
  }
  if (min !== undefined && num < min) {
    return `${label} must be at least ${min}.`
  }
  if (max !== undefined && num > max) {
    return `${label} must be at most ${max}.`
  }
  return null
}

export function validateCidrList(value: string, label = 'LAN Subnets'): string | null {
  const raw = value.trim()
  if (!raw) return null
  const items = raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  for (const cidr of items) {
    const [ip, prefixStr, extra] = cidr.split('/')
    if (!ip || !prefixStr || extra !== undefined) {
      return `${label}: "${cidr}" must use CIDR format (e.g. 192.168.10.0/24).`
    }
    if (!/^\d{1,3}$/.test(prefixStr)) {
      return `${label}: invalid prefix length in "${cidr}".`
    }
    const prefix = Number(prefixStr)
    if (isValidIPv4(ip)) {
      if (prefix < 0 || prefix > 32) return `${label}: IPv4 prefix in "${cidr}" must be 0 to 32.`
    } else if (isValidIPv6(ip)) {
      if (prefix < 0 || prefix > 128) return `${label}: IPv6 prefix in "${cidr}" must be 0 to 128.`
    } else {
      return `${label}: "${ip}" is not a valid IP address.`
    }
  }
  return null
}

export function validateHexColor(value: string, label = 'Color'): string | null {
  if (!HEX_RE.test(value.trim())) {
    return `${label} must be a 6-digit hex code (e.g. #0ea5e9).`
  }
  return null
}

export function firstError(errors: Record<string, string | null | undefined>): string | null {
  for (const val of Object.values(errors)) {
    if (val) return val
  }
  return null
}
