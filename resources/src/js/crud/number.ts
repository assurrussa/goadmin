// A number input retains the decimal literal even when converting it to a JS
// number would round it onto the bound. Compare those ties as decimal digits.
export function belowMinimum(value: number | string, minimum: number): boolean {
  const rounded = Number(value)
  if (typeof value === 'number' || rounded !== minimum) return rounded < minimum
  const input = decimalMagnitude(value)
  if (minimum === 0) return value.startsWith('-') && input.digits !== ''
  const bound = decimalMagnitude(String(minimum))
  const comparison = compareMagnitude(input, bound)
  return minimum < 0 ? comparison > 0 : comparison < 0
}

// A numeric JSON payload must retain the decimal value entered in the form.
export function exactWireNumber(raw: string): boolean {
  const number = Number(raw)
  if (!Number.isFinite(number)) return false
  const wire = String(number)
  if (raw === wire) return true
  const input = decimalMagnitude(raw)
  if (number === 0) return input.digits === ''
  return compareMagnitude(input, decimalMagnitude(wire)) === 0
}

function compareMagnitude(input: ReturnType<typeof decimalMagnitude>, bound: typeof input): number {
  if (input.order !== bound.order) return input.order - bound.order
  const width = Math.max(input.digits.length, bound.digits.length)
  const left = input.digits.padEnd(width, '0')
  const right = bound.digits.padEnd(width, '0')
  return left === right ? 0 : left < right ? -1 : 1
}

function decimalMagnitude(value: string) {
  const [coefficient, exponent = '0'] = value.toLowerCase().split('e')
  const [whole, fraction = ''] = coefficient!.replace(/^[-+]/, '').split('.')
  const digits = (whole! + fraction).replace(/^0+/, '')
  return { order: digits.length - fraction.length + Number(exponent), digits }
}
