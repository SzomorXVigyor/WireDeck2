import type { ModbusDataType } from '../types/webview'

export const MIN_PRECISION = -5
export const MAX_PRECISION = 5

export function clampPrecision(raw: unknown): number {
  const n = Math.trunc(Number(raw))
  if (!Number.isFinite(n)) return 0
  return Math.max(MIN_PRECISION, Math.min(MAX_PRECISION, n))
}

/**
 * Applies decimal exponent 10^-precision to a raw register value:
 *   precision = 0  -> raw as-is
 *   precision = 2  -> raw 1000 -> 10.00
 *   precision = -1 -> raw 5 -> 50
 */
export function decodePrecision(raw: number, precision: number): number {
  const p = clampPrecision(precision)
  return raw * Math.pow(10, -p)
}

/**
 * Converts a display value back to the raw register numeric value.
 */
export function encodePrecision(display: number, precision: number, dataType?: ModbusDataType): number {
  const p = clampPrecision(precision)
  const raw = display * Math.pow(10, p)
  if (dataType === 'FLOAT32') {
    return raw
  }
  return Math.round(raw)
}

export function formatPrecisionValue(raw: number | null | undefined, precision: number): string {
  if (raw === null || raw === undefined || !Number.isFinite(raw)) {
    return '--'
  }
  const p = clampPrecision(precision)
  const decoded = decodePrecision(raw, p)
  const decimals = Math.max(0, p)
  return decoded.toFixed(decimals)
}

export function isSignedDataType(dataType?: ModbusDataType): boolean {
  return dataType === 'INT16' || dataType === 'INT32' || dataType === 'FLOAT32'
}

export function rawTypeBounds(dataType?: ModbusDataType): { min: number; max: number } {
  switch (dataType) {
    case 'BOOL':
      return { min: 0, max: 1 }
    case 'UINT16':
      return { min: 0, max: 65535 }
    case 'INT16':
      return { min: -32768, max: 32767 }
    case 'UINT32':
      return { min: 0, max: 4294967295 }
    case 'INT32':
      return { min: -2147483648, max: 2147483647 }
    case 'FLOAT32':
      return { min: -3.4e38, max: 3.4e38 }
    default:
      return { min: 0, max: 65535 }
  }
}

/**
 * Computes effective display-unit [min, max] bounds combining register dataType bounds
 * (scaled by 10^-precision) with optional card-level min/max overrides.
 */
export function effectiveDisplayBounds(
  dataType: ModbusDataType | undefined,
  precision: number,
  cardMin?: number,
  cardMax?: number,
): { min: number; max: number; typeMin: number; typeMax: number } {
  const rawBounds = rawTypeBounds(dataType)
  const p = clampPrecision(precision)
  const factor = Math.pow(10, -p)
  const typeMin = rawBounds.min * factor
  const typeMax = rawBounds.max * factor

  let min = typeMin
  let max = typeMax
  if (cardMin !== undefined && Number.isFinite(cardMin)) {
    min = Math.max(typeMin, cardMin)
  }
  if (cardMax !== undefined && Number.isFinite(cardMax)) {
    max = Math.min(typeMax, cardMax)
  }
  return { min, max, typeMin, typeMax }
}

/**
 * Normalizes a text numeric input (supports European comma decimal separator).
 */
export function normalizeNumericText(input: string, allowDecimal: boolean, allowNegative: boolean): string {
  let result = input.replace(/,/g, '.')
  if (!allowDecimal) {
    result = result.replace(/[^0-9-]/g, '').replace(/(?!^)-/g, '')
  } else {
    const firstDot = result.indexOf('.')
    if (firstDot !== -1) {
      result = result.slice(0, firstDot + 1) + result.slice(firstDot + 1).replace(/\./g, '')
    }
    result = result.replace(/[^0-9.-]/g, '').replace(/(?!^)-/g, '')
  }
  if (!allowNegative) {
    result = result.replace(/-/g, '')
  }
  return result
}

export function validateDisplayNumber(
  text: string,
  dataType: ModbusDataType | undefined,
  precision: number,
  cardMin?: number,
  cardMax?: number,
): { valid: boolean; value: number | null; error: string | null } {
  const trimmed = text.trim()
  if (!trimmed || trimmed === '-' || trimmed === '.' || trimmed === '-.') {
    return { valid: false, value: null, error: 'Enter a numeric value' }
  }
  const num = Number(trimmed)
  if (!Number.isFinite(num)) {
    return { valid: false, value: null, error: 'Invalid number' }
  }
  const p = clampPrecision(precision)
  const signed = isSignedDataType(dataType)
  if (!signed && num < 0) {
    return { valid: false, value: null, error: 'Negative values are not allowed for unsigned register types' }
  }
  if (dataType !== 'FLOAT32' && p <= 0 && !Number.isInteger(num)) {
    return { valid: false, value: null, error: 'Value must be a whole number' }
  }
  if (p < 0) {
    const step = Math.pow(10, -p)
    const rem = Math.abs(num % step)
    if (rem > 1e-9 && Math.abs(rem - step) > 1e-9) {
      return { valid: false, value: null, error: `Value must be a multiple of ${step}` }
    }
  }
  const bounds = effectiveDisplayBounds(dataType, p, cardMin, cardMax)
  if (num < bounds.min - 1e-9) {
    return { valid: false, value: null, error: `Value must be at least ${bounds.min}` }
  }
  if (num > bounds.max + 1e-9) {
    return { valid: false, value: null, error: `Value must be at most ${bounds.max}` }
  }
  return { valid: true, value: num, error: null }
}
