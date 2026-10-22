const assert = require('node:assert/strict')
const postcss = require('postcss')

// CSS layer precedence is established by first declaration, even when that
// declaration comes from an extension before Tailwind's canonical prelude.
function verifyThemeLayerOrder(css) {
  const layers = []
  postcss.parse(css).walkAtRules('layer', (rule) => {
    for (let parent = rule.parent; parent; parent = parent.parent) {
      if (parent.type === 'atrule' && parent.name === 'layer') return
    }
    for (const name of rule.params.split(',').map((value) => value.trim().split('.')[0])) {
      if (name && !layers.includes(name)) layers.push(name)
    }
  })
  const semantic = layers.filter((name) =>
    ['theme', 'base', 'components', 'utilities'].includes(name),
  )
  assert.deepEqual(
    semantic,
    ['theme', 'base', 'components', 'utilities'],
    `admin CSS layer precedence must be theme, base, components, utilities; got ${layers.join(', ')}`,
  )
  return layers
}

module.exports = { verifyThemeLayerOrder }
