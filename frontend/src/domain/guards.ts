// Runtime shape guards shared by the domain rules. The API is a separate origin
// written in another language, so every payload is treated as unknown until it
// proves otherwise; these are the only tests the modules apply before reading a
// field, and they fail closed by returning false instead of coercing.

export function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

export function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}
