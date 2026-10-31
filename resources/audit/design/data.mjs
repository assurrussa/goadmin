// Synthetic presentation records shaped from the real page props and queue handlers.
const names = [
  ['Анна', 'Волкова', 'anna.volkova'],
  ['Михаил', 'Орлов', 'mikhail.orlov'],
  ['Елена', 'Соколова', 'elena.sokolova'],
  ['Дмитрий', 'Лебедев', 'dmitry.lebedev'],
  ['Мария', 'Козлова', 'maria.kozlova'],
  ['Алексей', 'Новиков', 'alexey.novikov'],
  ['Ольга', 'Морозова', 'olga.morozova'],
  ['Иван', 'Петров', 'ivan.petrov'],
]
export const userRows = names.map(([name, lastName, email], i) => ({
  item: {
    id: 1048 - i,
    name: `${name} ${lastName}`,
    email: `${email}@example.invalid`,
    status: i === 3 ? 'inactive' : 'active',
    createdAt: `2026-10-${String(21 - i).padStart(2, '0')}`,
  },
  actions: [
    { key: 'view', label: 'Посмотреть', icon: 'eye' },
    { key: 'edit', label: 'Редактировать', icon: 'edit' },
  ],
}))
const userColumns = [
  { key: 'id', label: 'ID', title: 'ID', type: 'number', sortable: true },
  { key: 'name', label: 'Пользователь', title: 'Пользователь', sortable: true },
  { key: 'email', label: 'Email', title: 'Email' },
  {
    key: 'status',
    label: 'Статус',
    title: 'Статус',
    type: 'select',
    filterable: true,
    options: [
      { value: 'active', label: 'Активен' },
      { value: 'inactive', label: 'Неактивен' },
    ],
    badges: {
      active: { label: 'Активен', variant: 'success' },
      inactive: { label: 'Неактивен', variant: 'secondary' },
    },
  },
  { key: 'createdAt', label: 'Регистрация', title: 'Регистрация', type: 'date', sortable: true },
]
const menu = [
  { name: 'Главная', href: '/', icon: 'home' },
  { name: 'Пользователи', href: '/users', icon: 'users' },
  { name: 'Администраторы', href: '/admins', icon: 'users' },
  { name: 'Роли', href: '/roles', icon: 'roles' },
  { name: 'Права доступа', href: '/permissions', icon: 'permissions' },
  { name: 'Очереди', href: '/queues', icon: 'queues' },
  { name: 'Уведомления', href: '/notifications', icon: 'notifications' },
]

// Every destination stays in this synthetic fixture; none can submit a mutation.
export const longMenu = Array.from({ length: 20 }, (_, index) => {
  const href = `/__design/long-menu/section-${index + 1}`
  return {
    name: `Раздел ${index + 1}: управление рабочими процессами`,
    href,
    icon: 'content',
    children: Array.from({ length: 2 }, (_, group) => ({
      name: `Группа ${group + 1}: настройки и наблюдение`,
      href: `${href}/group-${group + 1}`,
      children: Array.from({ length: 2 }, (_, leaf) => ({
        name: `Пункт ${leaf + 1}: подробная информация`,
        href: `${href}/group-${group + 1}/item-${leaf + 1}`,
      })),
    })),
  }
})

export const permissions = [
  ['users', 'read', 'Просмотр пользователей'],
  ['users', 'update', 'Редактирование пользователей'],
  ['roles', 'read', 'Просмотр ролей'],
  ['roles', 'assign', 'Назначение ролей'],
  ['queues', 'read', 'Просмотр задач'],
  ['notifications', 'read', 'Просмотр уведомлений'],
].map(([domain, action, description], index) => ({
  id: index + 1,
  uuid: `00000000-0000-4000-8000-${String(index + 1).padStart(12, '0')}`,
  domain,
  action,
  description,
}))
export const profile = {
  id: 1,
  name: 'Демо',
  lastName: 'Администратор',
  username: 'demo.admin',
  email: 'admin@example.invalid',
  createdAt: '2026-01-12T09:30:00Z',
  updatedAt: '2026-10-21T11:10:00Z',
  data: { roles: ['admin', 'content_editor'] },
}
export const admins = [
  profile,
  {
    id: 2,
    name: 'Анна',
    lastName: 'Волкова',
    username: 'anna.volkova',
    email: 'anna.volkova@example.invalid',
  },
]
export const roles = [
  {
    id: 1,
    slug: 'admin',
    name: 'Администратор',
    description: 'Управление доступом и задачами',
    isSystem: true,
    permissions,
    admins,
  },
  {
    id: 2,
    slug: 'content_editor',
    name: 'Редактор',
    description: 'Просмотр и редактирование пользователей',
    isSystem: false,
    permissions: permissions.slice(0, 2),
    admins: [admins[1]],
  },
]
export const notifications = ['success', 'warning', 'error', 'info'].map((level, index) => ({
  id: index + 1,
  adminId: 1,
  level,
  isRead: index % 2 === 1,
  title: [
    'Экспорт завершён',
    'Очередь требует внимания',
    'Не удалось доставить сообщение',
    'Обновление каталога',
  ][index],
  message: [
    'Файл с учётными записями подготовлен.',
    'Некоторые задачи ожидают повторного запуска.',
    'Проверьте состояние подключения и параметры получателя.',
    'Данные обновлены. Изменения доступны в панели управления.',
  ][index],
  createdAt: `2026-10-21T${String(14 - index).padStart(2, '0')}:30:00Z`,
  readAt: index % 2 === 1 ? '2026-10-21T15:00:00Z' : null,
  payload: {
    url: index === 1 ? '/queues/jobs-failed' : '/users',
    ctaText: 'Перейти',
    source: 'Synthetic design fixture',
  },
}))
const jobs = ['admin_notification', 'admin_preview_detach', 'users_export'].map((name, index) => ({
  id: 701 + index,
  queue: index === 1 ? 'uploads' : 'default',
  name,
  attempts: index,
  reservedAt: index === 0 ? '2026-10-21T14:30:00Z' : null,
  availableAt: '2026-10-21T14:30:00Z',
  createdAt: '2026-10-21T14:25:00Z',
  payloadPreview: '{"synthetic":true,"entityId":1048}',
}))
const failedJobs = jobs.slice(0, 2).map((job, index) => ({
  ...job,
  id: 801 + index,
  reason: index ? 'attempts_exhausted' : 'delivery_failed',
  exceptionPreview: index
    ? 'Retry limit reached after transient transport failures'
    : 'Synthetic gateway timeout: no delivery attempted',
  failedAt: '2026-10-21T14:35:00Z',
}))
const column = (key, label, extra = {}) => ({ key, label, title: label, ...extra })
const permissionColumns = [
  column('id', 'ID', { type: 'number', sortable: true }),
  column('domain', 'Раздел', { sortable: true }),
  column('action', 'Действие'),
  column('description', 'Описание'),
]
const roleColumns = [
  column('id', 'ID', { type: 'number', sortable: true }),
  column('slug', 'Slug'),
  column('name', 'Название', { sortable: true }),
  column('description', 'Описание'),
  column('isSystem', 'Системная', { type: 'boolean' }),
]
const queueColumns = [
  column('id', 'ID', { type: 'number', sortable: true }),
  column('queue', 'Очередь', { sortable: true }),
  column('name', 'Задача'),
  column('attempts', 'Попытки', { type: 'number' }),
  column('reservedAt', 'Зарезервировано', { type: 'date' }),
  column('availableAt', 'Доступно', { type: 'date' }),
  column('createdAt', 'Создано', { type: 'date' }),
  column('payloadPreview', 'Payload'),
]
const failedColumns = [
  column('id', 'ID', { type: 'number' }),
  column('name', 'Задача'),
  column('queue', 'Очередь'),
  column('reason', 'Причина'),
  column('exceptionPreview', 'Ошибка'),
  column('failedAt', 'Провалено', { type: 'date' }),
  column('createdAt', 'Создано', { type: 'date' }),
  column('payloadPreview', 'Payload'),
]
const wrap = (item, actions = []) => ({ item, actions })
export const gridRoutes = [
  '/users',
  '/permissions',
  '/roles',
  '/queues/jobs',
  '/queues/jobs-failed',
]

export function gridData(route, url) {
  const query = new URL(url, 'http://127.0.0.1').searchParams
  const scenario = query.get('scenario') || ''
  let columns = userColumns
  let title = 'Пользователи'
  let data = structuredClone(userRows)
  if (route === '/permissions') {
    columns = permissionColumns
    title = 'Права доступа'
    data = permissions.map((item) => wrap(item))
  } else if (route === '/roles') {
    columns = roleColumns
    title = 'Роли'
    data = roles.map((item) =>
      wrap(item, [
        { key: 'view', label: 'Посмотреть', icon: 'eye' },
        { key: 'edit', label: 'Редактировать', icon: 'edit' },
      ]),
    )
  } else if (route === '/queues/jobs') {
    columns = queueColumns
    title = 'Очередь задач'
    data = jobs.map((item) =>
      wrap(item, [{ key: 'delete', label: 'Удалить', icon: 'trash', variant: 'danger' }]),
    )
  } else if (route === '/queues/jobs-failed') {
    columns = failedColumns
    title = 'Проблемные задачи'
    data = failedJobs.map((item) =>
      wrap(item, [
        { key: 'retry', label: 'Повторить', icon: 'arrow-path', variant: 'primary' },
        { key: 'delete', label: 'Удалить', icon: 'trash', variant: 'danger' },
      ]),
    )
  }
  if (route.startsWith('/queues')) {
    data = data.map((row) => ({
      ...row,
      values: Object.fromEntries(
        columns
          .filter((column) => column.type === 'date')
          .map((column) => [
            column.key,
            row.item[column.key]
              ? String(row.item[column.key]).replace('T', ' ').slice(0, 16)
              : '—',
          ]),
      ),
    }))
  }
  if (route === '/users' && scenario === 'empty') data = []
  if (route === '/users' && scenario === 'long') {
    data[0].item.name = 'Александра-Екатерина Константинопольская-Добровольская'
    data[0].item.email =
      'alexandra.ekaterina.konstantinopolskaya.dobrovolskaya+administration@example.invalid'
    data[1].item.name = 'Alexander Christopher Montgomery-Wellington'
  }
  const search = query.get('search') || ''
  const status = query.get('status') || ''
  data = data.filter(
    (row) =>
      (!status || row.item.status === status) &&
      (!search || Object.values(row.item).join(' ').toLowerCase().includes(search.toLowerCase())),
  )
  const sortBy = query.get('sortBy') || 'id'
  const sortOrder = query.get('sortOrder') === 'asc' ? 'asc' : 'desc'
  data.sort(
    (a, b) =>
      String(a.item[sortBy] ?? '').localeCompare(String(b.item[sortBy] ?? ''), 'ru', {
        numeric: true,
      }) * (sortOrder === 'asc' ? 1 : -1),
  )
  const total = data.length
  const limit = Math.max(1, Math.min(100, Number(query.get('limit')) || 20))
  const page = Math.max(1, Number(query.get('page')) || 1)
  data = data.slice((page - 1) * limit, page * limit)
  const filters = Object.fromEntries(
    [...query].filter(([key]) => !['page', 'limit', 'sortBy', 'sortOrder', 'search'].includes(key)),
  )
  if (search) filters._search = search
  return {
    data,
    config: {
      routePath: route,
      columns,
      behaviour: {
        searchable: !route.startsWith('/queues'),
        creatable: ['/users', '/roles'].includes(route),
        selectable: route === '/users',
        refreshable: true,
        exportable: route === '/users',
      },
      ui: {
        createButtonText: route === '/roles' ? 'Создать роль' : 'Создать пользователя',
        idKey: 'id',
        emptyMessage: 'Нет данных для отображения',
      },
    },
    meta: {
      title,
      description: route === '/users' ? 'Учётные записи и доступ к приложению' : undefined,
      requestQuery: query.toString(),
      filters,
      sorting: { sortBy, sortOrder },
      pagination: {
        currentPage: page,
        perPage: limit,
        total,
        totalPages: Math.max(1, Math.ceil(total / limit)),
        from: total ? (page - 1) * limit + 1 : 0,
        to: Math.min(page * limit, total),
        links: [{ url: null, label: String(page), active: true }],
      },
    },
  }
}

export function makePage(url) {
  const { pathname, searchParams } = new URL(url, 'http://127.0.0.1')
  const page = {
    component: 'users/IndexPage',
    url,
    version: 'design-audit',
    props: {
      title: 'Пользователи',
      adminCapabilities: { notifications: true, authmail: true },
      adminInfo: { version: '0.13.0 · демо' },
      appearance: { owner: String(profile.id), version: 0, payload: { schemaVersion: 1, theme: 'system', locale: 'ru', density: 'default' } },
      branding: { version: 0, name: '', accentColor: '', logoLightUrl: '', logoDarkUrl: '' },
      appearanceAccess: { brandingRead: true, brandingUpdate: true, readOnly: false },
      flash: {},
      errors: {},
      old: {},
      authuser: { ...profile, roles: ['admin'], avatarUrl: '' },
      adminMenu: { sections: [{ key: 'main', title: 'Управление', items: menu }] },
      adminBreadcrumbs: [{ name: 'Главная', href: '/' }, { name: 'Пользователи' }],
    },
  }
  if (gridRoutes.includes(pathname)) {
    page.props.data = gridData(pathname, url)
    page.props.title = page.props.data.meta.title
    if (pathname === '/permissions') page.component = 'permissions/IndexPage'
    if (pathname === '/roles') page.component = 'roles/IndexPage'
    if (pathname === '/queues/jobs' || pathname === '/queues/jobs-failed') {
      page.component = pathname.endsWith('jobs-failed') ? 'queues/FailedPage' : 'queues/ActivePage'
      page.props.title = pathname.endsWith('jobs-failed') ? 'DLQ / Ошибки' : 'Активные задачи'
      page.props.jobsCount = jobs.length
      page.props.failedCount = failedJobs.length
    }
    if (
      pathname === '/users' &&
      ['loading', 'error', 'recovery'].includes(searchParams.get('scenario'))
    )
      page.props.data = { config: { routePath: '/users' } }
  } else if (pathname === '/__design/users-empty') {
    page.props.data = gridData('/users', '/users?scenario=empty')
    page.props.data.meta.filters = {}
    page.props.data.meta.requestQuery = ''
  } else if (pathname === '/') {
    page.component = 'dashboard/IndexPage'
    page.props.title = 'Главная'
  } else if (pathname === '/auth/login') {
    page.component = 'auth/LoginPage'
    page.props.title = 'Вход в систему'
    page.props.authuser = null
  } else if (pathname === '/auth/mfa') {
    page.component = 'auth/MFAPage'
    page.props.title = 'Подтверждение входа'
    page.props.authuser = null
    page.props.challengeID = 'synthetic-design-challenge-not-a-credential'
  } else if (pathname === '/auth/forgot-password') {
    page.component = 'auth/ForgotPasswordPage'
    page.props.title = 'Восстановление пароля'
    page.props.authuser = null
    page.props.formAction = '/auth/forgot-password'
    page.props.loginURL = '/auth/login'
  } else if (pathname === '/auth/reset-password') {
    page.component = 'auth/ResetPasswordPage'
    page.props.title = 'Сброс пароля'
    page.props.authuser = null
    page.props.token = 'synthetic-design-token-not-a-credential'
    page.props.email = 'admin@example.invalid'
    page.props.formAction = '/auth/reset-password'
    page.props.loginURL = '/auth/login'
  } else if (pathname === '/users/1048/edit' || pathname === '/users/create') {
    page.component = 'users/EditPage'
    page.props.title = 'Редактирование пользователя'
    page.props.data = {
      id: 1048,
      name: 'Анна',
      lastName: 'Волкова',
      username: 'anna.volkova',
      email: 'anna.volkova@example.invalid',
    }
  } else if (pathname === '/roles/1' || pathname === '/roles/1/edit') {
    page.component = pathname.endsWith('/edit') ? 'roles/EditPage' : 'roles/ViewPage'
    page.props.title = 'Роль администратора'
    page.props.role = roles[0]
    page.props.permissions = permissions
  } else if (pathname.startsWith('/auth/profile')) {
    page.component = pathname.endsWith('/settings')
      ? 'profile/SettingsPage'
      : pathname.endsWith('/edit')
        ? 'profile/EditPage'
        : 'profile/ViewPage'
    page.props.title = pathname.endsWith('/settings') ? 'Настройки профиля' : 'Профиль'
    page.props.data = profile
    page.props.mfaState = 'disabled'
    page.props.mfaAvailable = true
    page.props.mfaEnabled = false
  } else if (pathname === '/notifications') {
    page.component = 'notifications/IndexPage'
    page.props.title = 'Уведомления'
    const status = searchParams.get('status') || 'all'
    page.props.notifications =
      status === 'unread' ? notifications.filter((n) => !n.isRead) : notifications
    page.props.meta = {
      total: page.props.notifications.length,
      unreadCount: 2,
      page: 1,
      perPage: 10,
      totalPages: 1,
      hasMore: false,
    }
    page.props.filters = { status, page: 1, perPage: 10 }
  } else if (pathname === '/queues') {
    page.component = 'queues/IndexPage'
    page.props.title = 'Очереди задач'
    page.props.stats = { jobs: jobs.length, failed: failedJobs.length }
  } else if (pathname === '/__design/dialog') {
    page.component = 'DesignDialog'
    page.props.title = 'Настройки пользователя'
  } else if (pathname === '/__design/components') {
    page.component = 'DesignComponents'
    page.props.title = 'Компоненты'
  } else if (pathname === '/__design/long-menu' || pathname.startsWith('/__design/long-menu/')) {
    page.component = 'dashboard/IndexPage'
    page.props.title = 'Длинное вложенное меню'
    page.props.adminMenu = {
      sections: structuredClone(longMenu).map((item, index) => ({
        key: `long-${index + 1}`,
        title: `Длинный раздел навигации ${index + 1}`,
        items: [item],
      })),
    }
  } else if (pathname === '/__design/capabilities-limited') {
    page.component = 'dashboard/IndexPage'
    page.props.title = 'Ограниченные возможности'
    page.props.adminCapabilities = {}
    // The host owns permission/module filtering. The browser receives only the
    // allowed links; hiding a control is never treated as endpoint authorization.
    page.props.adminMenu.sections[0].items = menu.filter(
      (item) =>
        !['/notifications', '/queues', '/admins', '/roles', '/permissions'].includes(item.href),
    )
  } else if (pathname === '/__design/missing-menu') {
    page.component = 'dashboard/IndexPage'
    page.props.title = 'Меню не передано'
    page.props.adminCapabilities = {}
    delete page.props.adminMenu
  } else return null
  if (pathname !== '/users')
    page.props.adminBreadcrumbs = [
      { name: 'Главная', href: '/' },
      ...(pathname === '/' ? [] : [{ name: page.props.title }]),
    ]
  if (!page.props.authuser) {
    page.props.appearance = null
    page.props.appearanceAccess = { brandingRead: false, brandingUpdate: false, readOnly: false }
  }
  return page
}
