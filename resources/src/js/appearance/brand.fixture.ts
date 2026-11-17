// Literal stock tokens for DOM tests. The real stylesheet also uses layers and
// variable references that jsdom does not evaluate.
export function installStockBrandStyles() {
  const stock = (dark: boolean) => {
    const tone = (value: number, alpha = '') => `hsl(217 69% ${value}%${alpha})`
    const primary = tone(dark ? 74 : 46)
    const foreground = dark ? 'hsl(220 18% 12%)' : 'hsl(0 0% 100%)'
    return `
      --color-primary: ${primary};
      --color-primary-light: ${tone(dark ? 80 : 58)};
      --color-primary-lighter: ${tone(dark ? 86 : 70)};
      --color-primary-dark: ${tone(dark ? 64 : 36)};
      --color-primary-darker: ${tone(dark ? 54 : 28)};
      --color-primary-contrast: ${foreground};
      --color-primary-foreground: ${foreground};
      --color-primary-rgb: ${dark ? '143, 178, 234' : '37, 99, 200'};
      --color-ring: ${primary};
      --color-focus-ring: ${primary};
      --color-border-focus: ${primary};
      --outline-focus: 2px solid ${primary};
      --color-badge-primary-bg: ${dark ? tone(26, ' / 0.5') : tone(94, ' / 0.9')};
      --color-badge-primary-border: ${dark ? tone(52, ' / 0.45') : tone(46, ' / 0.35')};
      --color-badge-primary-text: ${tone(dark ? 76 : 38)};
    `
  }
  const sheet = document.createElement('style')
  sheet.textContent = `html { ${stock(false)} } html.dark { ${stock(true)} }`
  document.head.append(sheet)
  return () => sheet.remove()
}
