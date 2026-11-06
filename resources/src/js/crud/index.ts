import { markRaw, type Component } from 'vue'
import { recordListHref } from '@/composables/useRecordNavigation'

export interface CrudField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'number' | 'boolean' | 'date' | 'select' | 'custom'
  read: boolean
  create: boolean
  update: boolean
  required?: boolean
  options?: Array<{ value: string | number; label: string }>
  component?: string
}
export type CrudValue = string | number | boolean | null
export function crudValue(value: unknown): CrudValue {
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
    ? value
    : null
}
export interface CrudFieldProps {
  field: CrudField
  modelValue: CrudValue
  error?: string
  disabled: boolean
  readonly: boolean
}
export interface CrudResource {
  key: string
  basePath: string
  title: string
  fields: CrudField[]
  actions: Array<{ key: string; label: string; confirmation: string }>
}
export interface CrudRecord {
  id: string
  revision: string
  data: Record<string, unknown>
  actions?: string[]
}
export interface CrudPermissions {
  create: boolean
  update: boolean
  delete: boolean
}
const fields = new Map<string, Component<CrudFieldProps>>()
/** Register in AdminExtension.install. Keys are owned by the matching client extension. */
export function registerCrudField(key: string, component: Component<CrudFieldProps>): () => void {
  if (!/^[a-z][a-z0-9._-]*\/[a-z][a-z0-9._-]*$/.test(key) || fields.has(key))
    throw new Error(`Invalid or duplicate CRUD field: ${key}`)
  const value = markRaw(component)
  fields.set(key, value)
  return () => {
    if (fields.get(key) === value) fields.delete(key)
  }
}
export function resolveCrudField(key: string | undefined) {
  return key ? fields.get(key) : undefined
}
export function crudReturnHref(basePath: string, pageURL: string): string {
  return recordListHref(
    basePath,
    new URL(pageURL, 'https://goadmin.invalid').searchParams.get('returnTo'),
  )
}
export function crudRecordHref(
  basePath: string,
  id: string,
  edit = false,
  returnTo = basePath,
): string {
  const source = recordListHref(basePath, returnTo)
  return `${basePath}/records/${encodeURIComponent(id)}${edit ? '/edit' : ''}${source === basePath ? '' : `?${new URLSearchParams({ returnTo: source })}`}`
}
export function crudDisplay(field: CrudField, value: unknown): string {
  if (value === null || value === undefined) return '—'
  const option = field.options?.find((option) => String(option.value) === String(value))
  if (option) return option.label
  if (typeof value === 'object') return '—'
  return String(value)
}
