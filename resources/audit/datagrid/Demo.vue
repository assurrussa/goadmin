<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
defineOptions({ name: 'DataGridAuditDemo' })
import DataGrid from '../../src/js/components/datagrid/DataGrid.vue'
import type { ApiResponse, DataItemValue } from '../../src/js/composables/useDataGrid'
import { makeRows, MockGridHost, type Permission, type Trace } from './fixture'
const trace = ref<Trace[]>([])
const totals = ref<Record<string, number>>({})
let sequence = 0
const record = (kind: string, ...args: unknown[]) => {
  totals.value[kind] = (totals.value[kind] || 0) + 1
  trace.value.push({
    sequence: ++sequence,
    at: Math.round(performance.now()),
    kind,
    args: JSON.parse(JSON.stringify(args)),
  })
  if (trace.value.length > 300) trace.value.shift()
}
const host = new MockGridHost(record)
const total = ref(1000)
const pageSize = ref(100)
const permission = ref<Permission>('mixed')
const latency = ref(50)
const mountKey = ref(0)
const gridRef = ref<InstanceType<typeof DataGrid> | null>(null)
const initial = ref<ApiResponse | null>(host.response())
const bootstrap = ref<'prop' | 'network' | 'global'>('prop')
const syncWithUrl = ref(false)
const gridVisible = ref(true)
const form = ref<{ mode: 'create' | 'edit'; row: DataItemValue } | null>(null)
const formError = ref('')
const detail = ref<DataItemValue | null>(null)
const timings = ref<unknown[]>([])
const nativeFetch = window.fetch
window.fetch = host.fetch
onBeforeUnmount(() => {
  window.fetch = nativeFetch
})
const counts = computed(() =>
  Object.entries(totals.value)
    .map(([key, n]) => `${key}: ${n}`)
    .join(' · '),
)
function toggleGrid() {
  gridVisible.value = !gridVisible.value
  record('fixture.mount', gridVisible.value)
}
function clearTrace() {
  trace.value = []
  totals.value = {}
}
function cancelForm() {
  record('host.form.cancel', form.value?.mode)
  form.value = null
}
function closeDetail() {
  record('host.detail.close')
  detail.value = null
}
function apply() {
  host.rows = makeRows(total.value)
  host.pageSize = pageSize.value
  host.permission = permission.value
  host.latency = latency.value
  delete window.AdminDataGrid
  if (bootstrap.value === 'global') window.AdminDataGrid = JSON.stringify(host.response())
  initial.value = bootstrap.value === 'prop' ? host.response() : null
  mountKey.value++
  form.value = null
  detail.value = null
  record('fixture.reset', {
    total: total.value,
    pageSize: pageSize.value,
    permission: permission.value,
    latency: latency.value,
  })
}
function create() {
  record('callback.action-create')
  form.value = { mode: 'create', row: { name: '', amount: 0 } }
  formError.value = ''
}
function table(key: string, value: string | number | null) {
  record('callback.action-table', key, value)
  if (key === 'refresh') gridRef.value?.refreshData()
  if (key === 'export')
    record('host.export.preview', {
      rows: host.rows.length,
      format: 'json',
      note: 'Host simulation only; DataGrid has no built-in exporter.',
    })
}
function row(key: string, item: DataItemValue) {
  record('callback.action-row', key, item)
  if (key === 'edit') {
    form.value = { mode: 'edit', row: { ...item } }
    formError.value = ''
  } else if (key === 'view' || key === 'show') detail.value = item
  else if (key === 'delete') {
    record('host.delete.requested', item.id)
    detail.value = {
      note: 'Simulation records destructive intent; use Reset to restore fixture.',
      ...item,
    }
  } else record(`host.${key}.requested`, item.id)
}
function submit() {
  if (!form.value) return
  const error = host.submit(form.value.row, form.value.mode)
  formError.value = error?.name || ''
  if (!error) {
    form.value = null
    gridRef.value?.refreshData()
  }
}
function click(event: MouseEvent) {
  const target = event.target as HTMLElement
  const control = target.closest('button, input, th, checkbox, [role="menuitem"]')
  const tr = target.closest('tbody tr')
  record('dom.click', {
    tag: control?.tagName || target.tagName,
    label: control?.getAttribute('aria-label') || control?.textContent?.trim().slice(0, 100) || '',
    row: tr?.querySelector('td:nth-child(2)')?.textContent || null,
  })
}
function fail(status: number) {
  host.nextStatus = status
  record('fixture.next-status', status)
  gridRef.value?.refreshData()
}
async function race() {
  host.latency = 600
  gridRef.value?.refreshData()
  host.latency = 30
  gridRef.value?.refreshData()
  host.latency = latency.value
  record('fixture.race', 'older request waits 600ms; newer waits 30ms')
}
function injection() {
  host.rows = [
    {
      ...makeRows(1)[0],
      summary: JSON.stringify({
        type: 'doc',
        content: [
          {
            type: 'paragraph',
            content: [
              {
                type: 'text',
                text: '<b data-audit-injection="text-was-html">Synthetic HTML injection</b>',
              },
            ],
          },
        ],
      }),
    },
  ]
  initial.value = host.response()
  mountKey.value++
  record('fixture.injection', 'Harmless local HTML marker; no script, no network, no real data')
}
async function measure() {
  const samples: {
    rows: number
    milliseconds: number
    renderedRows: number
    domElements: number
  }[] = []
  const old = { rows: host.rows, size: host.pageSize }
  for (const size of [100, 1000]) {
    for (let run = 0; run < 4; run++) {
      gridVisible.value = false
      await nextTick()
      host.rows = makeRows(size)
      host.pageSize = size
      initial.value = host.response()
      const start = performance.now()
      gridVisible.value = true
      mountKey.value++
      await nextTick()
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      )
      const milliseconds = performance.now() - start
      if (run > 0)
        samples.push({
          rows: size,
          milliseconds: Math.round(milliseconds * 10) / 10,
          renderedRows: document.querySelectorAll('.data-grid tbody tr').length,
          domElements: document.querySelectorAll('.data-grid *').length,
        })
    }
  }
  timings.value = samples
  host.rows = old.rows
  host.pageSize = old.size
  initial.value = host.response()
  mountKey.value++
  record('fixture.render-timing', {
    scope:
      'Vue mount through two animation frames; dev build; one warmup and three samples per size; not API latency',
    browser: navigator.userAgent,
    samples,
  })
}
</script>

<template>
  <main class="lab" @click.capture="click">
    <section class="lab-panel">
      <h1 class="text-2xl font-bold mb-2">DataGrid contract lab</h1>
      <p class="mb-3">
        First-stage fixes on baseline 7d51c83. Real DataGrid from this branch. No database,
        credentials, external requests or persistent mutations. Every click, callback and mock
        request is logged.
      </p>
      <div class="lab-controls">
        <label
          >Total records
          <select v-model.number="total">
            <option :value="0">0</option>
            <option :value="100">100</option>
            <option :value="1000">1000</option>
          </select></label
        >
        <label
          >Initial fixture page size
          <select v-model.number="pageSize">
            <option :value="10">10</option>
            <option :value="100">100</option>
            <option :value="1000">1000</option>
          </select></label
        >
        <label
          >Permission fixture
          <select v-model="permission">
            <option value="full">Full</option>
            <option value="readonly">Read only</option>
            <option value="mixed">Mixed / actionless rows</option>
          </select></label
        >
        <label
          >Bootstrap
          <select v-model="bootstrap">
            <option value="prop">initialData prop</option>
            <option value="network">Mock fetch on mount</option>
            <option value="global">window.AdminDataGrid JSON</option>
          </select></label
        >
        <label>Sync URL on reset<input v-model="syncWithUrl" type="checkbox" /></label>
        <label>Delay, ms <input v-model.number="latency" type="number" min="0" max="2000" /></label>
        <button @click="apply">Apply / reset</button>
        <button @click="fail(401)">Next 401</button><button @click="fail(403)">Next 403</button
        ><button @click="fail(500)">Next 500</button>
        <button @click="race">Out-of-order responses</button>
        <button @click="injection">Safe HTML escaping probe</button>
        <button @click="measure">Measure 100 / 1000</button>
        <button @click="toggleGrid">
          {{ gridVisible ? 'Unmount grid' : 'Mount grid' }}
        </button>
      </div>
    </section>
    <section class="lab-panel lab-warning">
      <strong>Capability boundaries remain explicit.</strong> Selection keeps its existing page-load
      reset behavior; inline creation has no UI. Bulk actions, embedded forms, actual export, image
      and standalone badge renderers are not implemented by DataGrid. Form controls below simulate
      host callbacks. The real Go GET handler caps rows at 100; 1000-row rendering here is a fixture
      stress case, not a claim about that endpoint.
    </section>
    <section class="lab-panel" aria-label="Callback trace">
      <h2 class="font-bold">Event trace (last 300 events)</h2>
      <p class="my-2">{{ counts }}</p>
      <div class="lab-controls mb-2">
        <button @click.stop="clearTrace">Clear trace</button>
      </div>
      <pre class="lab-trace">{{
        trace.map((event) => JSON.stringify(event)).join('\n') ||
        'Interact with the grid to record callbacks.'
      }}</pre>
    </section>
    <section v-if="form" class="lab-panel">
      <h2 class="font-bold mb-2">Host form simulation: {{ form.mode }}</h2>
      <form class="lab-form" @submit.prevent="submit">
        <label>Name<input v-model="form.row.name" name="fixture-name" aria-label="Name" /></label>
        <label
          >Amount<input
            v-model.number="form.row.amount"
            type="number"
            name="fixture-amount"
            aria-label="Amount"
        /></label>
        <button type="submit">Save fixture</button
        ><button type="button" @click="cancelForm">Cancel</button>
        <p v-if="formError" role="alert">{{ formError }}</p>
      </form>
    </section>
    <section v-if="detail" class="lab-panel">
      <div class="lab-controls">
        <button @click="closeDetail">Close details</button>
      </div>
      <pre class="lab-trace">{{ JSON.stringify(detail, null, 2) }}</pre>
    </section>
    <section v-if="timings.length" class="lab-panel">
      <h2>Browser mount samples</h2>
      <pre>{{ JSON.stringify(timings, null, 2) }}</pre>
    </section>
    <DataGrid
      v-if="gridVisible"
      :key="mountKey"
      ref="gridRef"
      api-url="/audit"
      :initial-data="initial"
      :sync-with-url="syncWithUrl"
      @action-create="create"
      @action-table="table"
      @action-row="row"
      @page-change="(value) => record('callback.page-change', value)"
      @action-refresh="record('callback.action-refresh')"
      @action-export="record('callback.action-export')"
    />
  </main>
</template>
