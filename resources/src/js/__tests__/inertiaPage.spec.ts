import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { toInertiaPage } from '../inertiaPage'
import type { PageWithLayout } from '../inertiaPage'

describe('Inertia page type boundary', () => {
  it('preserves the typed component, props and persistent layout without a wrapper', () => {
    const component = defineComponent({
      props: { title: { type: String, required: true } },
      setup: (props) => () => h('h1', props.title),
    })
    const layout = defineComponent({ setup: () => () => h('main') })
    const page: PageWithLayout = Object.assign(component, { layout })

    expect(toInertiaPage(page)).toBe(component)
    expect(toInertiaPage(page).props).toBe(component.props)
    expect(toInertiaPage(page).layout).toBe(layout)
  })
})
