// A number input retains the decimal literal even when converting it to a JS
// number would round it onto the bound. Compare those ties as decimal digits.
export function belowMinimum(value: number | string, minimum: number): boolean {
  const rounded = Number(value)
  if (typeof value === 'number' || rounded !== minimum) return rounded < minimum
  const input = decimalMagnitude(value)
  if (minimum === 0) return value.startsWith('-') && input.digits !== ''
  const bound = decimalMagnitude(String(minimum))
  if (input.order !== bound.order)
    return minimum < 0 ? input.order > bound.order : input.order < bound.order
  const width = Math.max(input.digits.length, bound.digits.length)
  const left = input.digits.padEnd(width, '0')
  const right = bound.digits.padEnd(width, '0')
  return minimum < 0 ? left > right : left < right
}

function decimalMagnitude(value: string) {
  const [coefficient, exponent = '0'] = value.toLowerCase().split('e')
  const [whole, fraction = ''] = coefficient!.replace(/^[-+]/, '').split('.')
  const digits = (whole! + fraction).replace(/^0+/, '')
  return { order: digits.length - fraction.length + Number(exponent), digits }
}
