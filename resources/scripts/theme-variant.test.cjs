const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const test = require('node:test')

test('dark utilities follow the shell selectors instead of the operating system', async () => {
  const { compile } = await import('tailwindcss')
  // Compile the actual theme and variant declarations, without filesystem
  // imports or extension scanning. The normal Vite build covers those inputs.
  const source = fs
    .readFileSync(path.join(__dirname, '../src/css/app.css'), 'utf8')
    .replace(/^@import .*;$/gm, '')
  const compiler = await compile(`${source}\n@tailwind utilities;`)
  const css = compiler.build(['dark:bg-card', 'dark:text-card-foreground'])
  assert.match(css, /\.dark\\:bg-card/)
  assert.match(css, /\.dark\\:text-card-foreground/)
  assert.match(css, /:where\(\.dark, \.dark \*, \[data-theme-mode=['"]dark['"]\], \[data-theme-mode=['"]dark['"]\] \*\)/)
  assert.doesNotMatch(css, /prefers-color-scheme/)
})
