function hasField(data: Record<string, unknown>, key: string): boolean {
  // The "in" check lets Vue track later additions and deletions on reactive records.
  return key in data && Object.hasOwn(data, key)
}

export function recordFieldValue(
  data: Record<string, unknown>,
  key: string,
  overrides?: Record<string, unknown>,
): unknown {
  if (overrides && hasField(overrides, key)) return overrides[key]
  return hasField(data, key) ? data[key] : undefined
}
