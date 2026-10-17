import { afterEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { useRolePreviewAction } from './useRolePreviewAction'
const fixture = vi.hoisted(() => ({ page: null as unknown as { props: Record<string, unknown> } }))
vi.mock('@inertiajs/vue3', () => ({ usePage: () => fixture.page }))
afterEach(() => {
  fixture.page = { props: {} }
})
it('preserves ordinary pages and fails closed for unknown active preview actions', () => {
  fixture.page = reactive({ props: {} as Record<string, unknown> })
  const canEdit = useRolePreviewAction('admins', 'update')
  expect(canEdit.value).toBe(true)
  fixture.page.props.rolePreview = { active: false, actions: { 'admins:update': false } }
  expect(canEdit.value).toBe(true)
  fixture.page.props.rolePreview = { active: true }
  expect(canEdit.value).toBe(false)
  fixture.page.props.rolePreview = { active: true, actions: { 'admins:update': 'true' } }
  expect(canEdit.value).toBe(false)
  fixture.page.props.rolePreview = {
    active: true,
    actions: { 'admins:update': true, 'users:update': false },
  }
  expect(canEdit.value).toBe(true)
  expect(useRolePreviewAction('users', 'update').value).toBe(false)
  fixture.page.props.rolePreview = { active: false }
  expect(canEdit.value).toBe(true)
})
