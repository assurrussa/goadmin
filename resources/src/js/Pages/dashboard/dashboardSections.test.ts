import { describe, expect, it } from 'vitest'
import { dashboardSections } from './dashboardSections'

describe('dashboardSections', () => {
  it('shows only links supplied by the filtered menu, including nested links', () => {
    expect(
      dashboardSections({
        sections: [
          { key: 'main', items: [{ name: 'Главная', href: '/' }] },
          {
            key: 'work',
            title: 'Работа',
            items: [
              {
                name: 'Группа',
                children: [
                  { name: 'Очереди', href: '/queues' },
                  { name: 'External', href: 'https://example.com' },
                ],
              },
            ],
          },
        ],
      }),
    ).toEqual([{ key: 'work', title: 'Работа', links: [{ name: 'Очереди', href: '/queues' }] }])
  })

  it('returns no links when the server supplies no menu', () => {
    expect(dashboardSections()).toEqual([])
  })
})
