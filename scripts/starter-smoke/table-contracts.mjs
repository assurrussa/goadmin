import assert from 'node:assert/strict'

export const tableResources = ['/users', '/admins', '/roles', '/permissions', '/queues/jobs', '/queues/jobs-failed']

export async function tableExportMatrix({ context, get, prefix, parseCSV }) {
  const report = {}
  for (const resource of tableResources) {
    const query = new URLSearchParams({ limit: '100', sortBy: 'id', sortOrder: 'asc' })
    if (resource === '/users') { query.set('search', prefix); query.set('status', 'active') }
    if (resource.startsWith('/queues/')) query.set('name', prefix + (resource.endsWith('failed') ? 'failed' : 'job'))
    const snapshot = await get(`${resource}/data?${query}`)
    assert.ok(snapshot.data.length, `${resource} must have real export fixtures`)
    const columns = [...snapshot.config.columns].reverse()
    const ids = snapshot.data.slice(0, 2).map(row => row.selectionId)
    const idIndex = columns.findIndex(column => column.key === 'id')
    report[resource] = {}
    for (const scope of ['filtered', 'selected']) {
      const params = new URLSearchParams(query)
      params.set('scope', scope)
      params.set('columns', columns.map(column => column.key).join(','))
      if (scope === 'selected') params.set('ids', ids.join(','))
      const response = await context.request.get(`${snapshot.config.workflow.exportUrl}?${params}`)
      assert.equal(response.status(), 200, `${resource} ${scope}`)
      assert.match(response.headers()['content-type'], /text\/csv/)
      assert.equal(response.headers()['cache-control'], 'no-store')
      const text = await response.text()
      const csv = parseCSV(text)
      assert.deepEqual(csv[0], columns.map(column => column.label))
      assert.equal(csv.length - 1, scope === 'selected' ? ids.length : snapshot.meta.pagination.total)
      if (scope === 'selected' && idIndex >= 0) {
        assert.deepEqual(new Set(csv.slice(1).map(row => row[idIndex])), new Set(ids))
      }
      for (const [rowIndex, row] of csv.slice(1).entries()) {
        const expected = scope === 'selected' ? snapshot.data.find(item => item.selectionId === ids[rowIndex])
          : snapshot.data[rowIndex]
        if (!expected) continue // The filtered export can extend beyond this API page.
        for (let i = 0; i < columns.length; i++) {
          const key = columns[i].key
          const value = Object.hasOwn(expected.values ?? {}, key) ? expected.values[key] : expected.item[key]
          if (value === null || value === undefined) assert.equal(row[i], '', `${resource}/${key}`)
          else if (['string', 'boolean', 'number'].includes(typeof value)) {
            const raw = String(value)
            const safe = /^[\t\r\n]|^ *[=+\-@]/.test(raw) ? `'${raw}` : raw
            assert.equal(row[i], safe, `${resource}/${key}`)
          }
        }
      }
      if (resource.startsWith('/queues/')) {
        assert.ok(!text.includes('smoke-secret-token'), 'CSV must never contain the source payload')
        const payloadIndex = columns.findIndex(column => column.key === 'payloadPreview')
        assert.ok(payloadIndex >= 0)
        for (const row of csv.slice(1)) assert.equal(row[payloadIndex], 'Скрыто')
      }
      report[resource][scope] = { rows: csv.length - 1, columns: columns.map(column => column.key) }
    }
  }
  return report
}

// Requests use the real browser cookie jar and native session/CSRF/RBAC stack.
// No route stubs and no forged session locals are involved.
export async function tableSecurityMatrix({ page, context, config, credentials, get, prefix, login, logout }) {
  const snapshots = {}
  for (const resource of tableResources) {
    const conditions = resource === '/users' ? `&search=${prefix}`
      : resource.startsWith('/queues/') ? `&name=${prefix}${resource.endsWith('failed') ? 'failed' : 'job'}` : ''
    const grid = await get(`${resource}/data?limit=1${conditions}`)
    snapshots[resource] = { grid, prefs: await get(grid.config.workflow.preferencesUrl) }
  }
  const send = async (method, url, body, csrf = true, origin = config.url) => {
    const token = (await context.cookies(config.url)).find(cookie => cookie.name === 'csrf_token')?.value
    assert.ok(token)
    return context.request.fetch(url, { method, data: body, maxRedirects: 0,
      headers: { Accept: 'application/json', Origin: origin, Referer: `${config.url}/users`,
        ...(csrf ? { 'X-CSRF-Token': token } : {}) } })
  }
  const preferenceBody = prefs => ({ version: prefs.version, payload: prefs.payload })
  const mutations = tableResources.map(resource => ({ method: 'PUT', url: `${resource}/table-preferences`,
    body: preferenceBody(snapshots[resource].prefs) }))
  for (const [resource, action] of [['/users', 'delete'], ['/users', 'restore'], ['/queues/jobs', 'delete'],
    ['/queues/jobs-failed', 'delete'], ['/queues/jobs-failed', 'retry']]) {
    mutations.push({ method: 'POST', url: `${resource}/bulk`, body: { action, ids: [snapshots[resource].grid.data[0].selectionId] } })
  }
  const assertUnchanged = async () => {
    for (const resource of tableResources) assert.deepEqual(await get(`${resource}/table-preferences`), snapshots[resource].prefs)
    for (const resource of ['/users', '/queues/jobs', '/queues/jobs-failed']) {
      const row = snapshots[resource].grid.data[0]
      const response = await context.request.get(`${resource}/table-export?scope=selected&columns=id&ids=${row.selectionId}`)
      assert.equal(response.status(), 200)
      assert.ok((await response.text()).includes(row.selectionId))
    }
  }
  // Missing proof and a hostile Origin must fail before every new mutation endpoint.
  for (const mode of ['missing-csrf', 'foreign-origin']) {
    for (const mutation of mutations) {
      const response = await send(mutation.method, mutation.url, mutation.body,
        mode !== 'missing-csrf', mode === 'foreign-origin' ? 'https://foreign.example.test' : config.url)
      // The native Inertia error listener returns an error redirect; a valid command returns JSON 200.
      assert.equal(response.status(), 302, `${mode}: ${mutation.url}`)
      assert.equal(response.headers().location, `${config.url}/users`)
    }
    await assertUnchanged()
  }
  for (const resource of tableResources) {
    const column = snapshots[resource].grid.config.columns[0].key
    const invalid = await context.request.get(`${resource}/table-export?scope=selected&columns=${column}&ids=not-an-id`)
    assert.equal(invalid.status(), 400, `${resource}: malformed selected ID`)
    const forged = await send('PUT', `${resource}/table-preferences`, { ...preferenceBody(snapshots[resource].prefs), owner: '999999' })
    assert.equal(forged.status(), 400)
    const own = await get(`${resource}/table-preferences?owner=999999&namespace=other`)
    assert.deepEqual(own, snapshots[resource].prefs)
  }
  for (const mutation of mutations.filter(item => item.method === 'POST')) {
    const response = await send('POST', mutation.url, { ...mutation.body, ids: [...mutation.body.ids, 'not-an-id'] })
    assert.equal(response.status(), 400, 'whole bulk input is validated before touching the valid first ID')
  }
  await assertUnchanged()
  const roles = await get('/roles/data?limit=100')
  const superRole = roles.data.find(row => row.item.slug === 'super_admin')
  assert.ok(superRole)
  assert.equal((await send('POST', '/auth/role-preview', { roleId: Number(superRole.selectionId) })).status(), 200)
  try {
    for (const mutation of mutations) assert.equal((await send(mutation.method, mutation.url, mutation.body)).status(), 403, mutation.url)
    for (const resource of ['/users', '/queues/jobs', '/queues/jobs-failed']) {
      const grid = await get(`${resource}/data?limit=1`)
      assert.equal(grid.config.workflow.bulkActions?.length ?? 0, 0)
    }
    await assertUnchanged()
  } finally { assert.equal((await send('POST', '/auth/role-preview', { roleId: 0 })).status(), 200) }

  const emptySlug = `${prefix}no-access`
  const created = await send('POST', '/roles', { name: 'No table access', slug: emptySlug, permissions: [] })
  assert.equal(created.status(), 302)
  const emptyRoles = await get(`/roles/data?slug=${emptySlug}`)
  const emptyRole = emptyRoles.data.find(row => row.item.slug === emptySlug)
  assert.ok(emptyRole, 'empty role must be created by the actual role command')
  assert.equal((await send('POST', '/auth/role-preview', { roleId: Number(emptyRole.selectionId) })).status(), 200)
  try {
    for (const resource of tableResources) {
      for (const suffix of ['table-preferences', 'table-export?columns=id']) {
        const denied = await context.request.get(`${resource}/${suffix}`, { maxRedirects: 0 })
        assert.equal(denied.status(), 403, `empty role: ${resource}/${suffix}`)
      }
    }
    assert.equal((await context.request.get('/queues/stats', { maxRedirects: 0 })).status(), 403)
  } finally { assert.equal((await send('POST', '/auth/role-preview', { roleId: 0 })).status(), 200) }

  // The existing restricted account has only queues:read, with a distinct preferences owner.
  await logout(page)
  await login(page, credentials.restrictedEmail, credentials.password)
  const deniedMutation = async response => {
    assert.equal(response.status(), 302, 'native mutation guard uses the Inertia error redirect')
    assert.equal(response.headers().location, `${config.url}/users`)
    const landing = await context.request.get('/queues/jobs', { headers: { 'X-Inertia': 'true' } })
    assert.equal(landing.status(), 200)
    const { props } = await landing.json()
    assert.equal(props.flash?.error, 'Permission denied',
      'must be an RBAC denial, not a CSRF/session failure or a successful mutation redirect')
  }
  for (const resource of tableResources.filter(resource => !resource.startsWith('/queues/'))) {
    for (const suffix of ['table-preferences', 'table-export?columns=id']) {
      const response = await context.request.get(`${resource}/${suffix}`, { maxRedirects: 0 })
      assert.equal(response.status(), 403, `${resource}/${suffix}`)
    }
    await deniedMutation(await send('PUT', `${resource}/table-preferences`, preferenceBody(snapshots[resource].prefs)))
  }
  for (const mutation of mutations.filter(item => item.method === 'POST')) {
    await deniedMutation(await send(mutation.method, mutation.url, mutation.body))
  }
  for (const resource of ['/queues/jobs', '/queues/jobs-failed']) {
    const own = await get(`${resource}/table-preferences?owner=${snapshots[resource].prefs.owner}`)
    assert.notEqual(own.owner, snapshots[resource].prefs.owner)
    assert.equal(own.version, 0)
    assert.deepEqual(own.payload.views, [])
    const saved = await send('PUT', `${resource}/table-preferences`, preferenceBody(own))
    assert.equal(saved.status(), 200, 'read permission permits only personal preference changes')
    assert.equal((await saved.json()).owner, own.owner)
  }
  assert.equal((await context.request.get('/queues/stats')).status(), 200)
  await logout(page)
  await login(page, credentials.email, credentials.password)
  await assertUnchanged()
  return { csrfRoutes: mutations.length, previewRoutes: mutations.length, forgedInput: true,
    deniedReadAllResources: true, restrictedReadAndMutations: true, ownerIsolation: true }
}
