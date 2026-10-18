export const views = ['dashboard', 'participants', 'training', 'permissions', 'queue', 'settings']
export const presets = ['default', 'compact', 'branded']
export const seedDate = '2026-10-19'
export const participants = Array.from({ length: 24 }, (_, i) => ({
  id: `NC-${String(1041 + i)}`,
  name:
    [
      'Alex Morgan',
      'Jamie Park',
      'Robin Ellis',
      'Sam Rivers',
      'Taylor Quinn',
      'Casey Lane',
      'Drew Jordan',
      'Avery Reed',
    ][i % 8] + (i > 7 ? ` ${Math.floor(i / 8) + 1}` : ''),
  group: ['Performance', 'Community', 'Junior academy'][i % 3],
  status: i % 5 === 0 ? 'Pending' : 'Active',
  attendance: `${84 + (i % 15)}%`,
  joined: '12 Oct 2026',
}))
export const sessions = [
  ['07:00', 'Strength & conditioning', 'Performance', 'Studio A', '18 / 20', 'Ready'],
  ['09:30', 'Junior skills lab', 'Junior academy', 'Court 2', '14 / 16', 'Ready'],
  ['12:00', 'Community run', 'Community', 'Riverside', '24 / 30', 'Open'],
  ['17:30', 'Team tactics', 'Performance', 'Court 1', '20 / 20', 'Full'],
]
export function demoPage(url) {
  const parsed = new URL(url, 'http://127.0.0.1')
  const view = parsed.pathname.split('/').pop() || 'dashboard'
  if (!views.includes(view)) return null
  const preset = presets.includes(parsed.searchParams.get('preset'))
    ? parsed.searchParams.get('preset')
    : 'compact'
  const mode = parsed.searchParams.get('mode') === 'dark' ? 'dark' : 'light'
  const href = (v) => `/demo/${v}`
  return {
    component: 'SportsClubDemo',
    url: parsed.pathname + parsed.search,
    version: 'sports-club-v1',
    props: {
      view,
      preset,
      mode,
      adminCapabilities: {},
      adminInfo: { version: '0.13.0 · demo' },
      authuser: { id: 1, name: 'Demo administrator', email: 'admin@example.invalid', roles: [] },
      flash: {},
      adminMenu: {
        sections: [
          {
            key: 'club',
            title: 'NORTHLINE · SPORTS CLUB',
            items: views.map((v, i) => ({
              name: [
                'Overview',
                'Participants',
                'Training editor',
                'Roles & permissions',
                'Job queue',
                'Profile & settings',
              ][i],
              href: href(v),
              icon: ['home', 'users', 'content', 'roles', 'queues', 'sessions'][i],
            })),
          },
        ],
      },
    },
  }
}
