<script setup lang="ts">
import { computed, ref } from 'vue'
import AdminLayout from '@/components/layout/AdminLayout.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import {
  Users,
  Activity,
  CalendarDays,
  Target,
  ArrowUpRight,
  ShieldCheck,
  Bell,
  ListTodo,
} from 'lucide-vue-next'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { participants, sessions } from './data.mjs'
const notificationOpen = ref(false)
const administrators = ref([
  { name: 'Demo administrator', role: 'Administrator' },
  { name: 'Morgan Lee', role: 'Coach' },
  { name: 'Riley Chen', role: 'Front desk' },
])
const notifications = [
  'Attendance report is ready',
  '5 participant registrations need review',
  'Saturday training is nearly full',
]
const props = defineProps<{ view: string; preset: string; mode: string }>()
const titles: Record<string, string> = {
  dashboard: 'Club overview',
  participants: 'Participants',
  training: 'Training editor',
  permissions: 'Roles & permissions',
  queue: 'Job queue',
  settings: 'Profile & settings',
}
const query = ref(''),
  status = ref('All'),
  page = ref(1),
  notice = ref(''),
  title = ref('Strength & conditioning'),
  description = ref(
    'Build a stronger foundation. A coach-led session focused on movement quality, strength and sustainable progress.',
  ),
  role = ref('Coach'),
  rolePermissions = ref<Record<string, string[]>>({
    Coach: ['View participants', 'View training', 'Edit training'],
    Administrator: [
      'View participants',
      'Edit participants',
      'View training',
      'Edit training',
      'Manage roles',
      'View queue',
    ],
    'Front desk': ['View participants', 'View training'],
  })
const permissions = computed({
  get: () => rolePermissions.value[role.value] ?? [],
  set: (v: string[]) => {
    rolePermissions.value[role.value] = v
  },
})
const coach = ref('Morgan Lee (fictional)'),
  date = ref('2026-10-19'),
  time = ref('07:00'),
  capacity = ref(20),
  location = ref('Studio A'),
  displayName = ref('Demo administrator'),
  email = ref('admin@example.invalid')
const filtered = computed(() =>
  participants.filter(
    (p) =>
      p.name.toLowerCase().includes(query.value.toLowerCase()) &&
      (status.value === 'All' || p.status === status.value),
  ),
)
const rows = computed(() => filtered.value.slice((page.value - 1) * 6, page.value * 6))
const preview = (s: string) => (notice.value = s + ' · local preview only')
const href = (v: string) => `/demo/${v}?preset=${props.preset}&mode=${props.mode}`
const metrics = [
  { label: 'Active participants', value: '248', change: '+12 this month', icon: Users },
  { label: 'Weekly attendance', value: '87.4%', change: '+5.2% vs last week', icon: Activity },
  {
    label: 'Sessions this week',
    value: '32',
    change: 'Across 3 training groups',
    icon: CalendarDays,
  },
  { label: 'Capacity filled', value: '76%', change: '48 spaces available', icon: Target },
]
</script>
<template>
  <AdminLayout
    ><div class="club-demo" data-demo-ready>
      <div class="banner">
        <span>● &nbsp; NORTHLINE / CLUB OPERATIONS</span>
        <div class="capability-cues">
          <span class="seed-label">Synthetic · 19 Oct 2026</span
          ><a :href="href('queue')" class="queue-indicator" aria-label="Synthetic queue: 3 waiting"
            ><ListTodo :size="15" /><span>Queue</span><AppBadge variant="outline">3</AppBadge></a
          ><DropdownMenu v-model:open="notificationOpen"
            ><DropdownMenuTrigger as-child
              ><AppButton variant="outline" size="sm" aria-label="Synthetic notifications: 3 unread"
                ><Bell :size="15" /><span>3</span></AppButton
              ></DropdownMenuTrigger
            ><DropdownMenuContent align="end" style="width: min(22rem, calc(100vw - 2rem))"
              ><DropdownMenuLabel>Notifications · synthetic</DropdownMenuLabel
              ><DropdownMenuSeparator /><DropdownMenuItem
                v-for="(n, i) in notifications"
                :key="n"
                @select="preview(n)"
                ><span class="notification-item"
                  ><strong>{{ n }}</strong
                  ><small>{{ [12, 28, 45][i] }} min ago · local preview</small></span
                ></DropdownMenuItem
              ></DropdownMenuContent
            ></DropdownMenu
          >
        </div>
      </div>
      <div class="heading">
        <div>
          <p class="eyebrow">WORKSPACE / {{ titles[view] }}</p>
          <h1>{{ titles[view] }}</h1>
          <p class="muted">
            {{
              view === 'dashboard'
                ? 'A clear view of your club. Every session, every participant.'
                : 'A presentation workspace built with shared GoAdmin components.'
            }}
          </p>
        </div>
        <div class="actions">
          <AppBadge variant="outline">{{ preset }} · {{ mode }}</AppBadge
          ><AppButton @click="preview('Changes saved')"
            >{{ view === 'dashboard' ? 'Review week' : 'Save preview' }} <ArrowUpRight :size="15"
          /></AppButton>
        </div>
      </div>
      <p v-if="notice" class="notice" role="status">{{ notice }}</p>
      <template v-if="view === 'dashboard'">
        <div class="metrics">
          <AppCard v-for="m in metrics" :key="m.label" class="metric"
            ><div class="metric-label">{{ m.label }}<component :is="m.icon" :size="18" /></div>
            <strong>{{ m.value }}</strong>
            <p class="muted">↗ {{ m.change }}</p></AppCard
          >
        </div>
        <div class="dashboard-grid">
          <AppCard class="panel"
            ><div class="panel-heading">
              <div>
                <p class="eyebrow">ENGAGEMENT</p>
                <h2>Showing up, getting stronger</h2>
                <p class="muted">Weekly attendance · 12–18 October</p>
              </div>
              <AppBadge variant="outline">This week</AppBadge>
            </div>
            <div class="chart-summary">
              <strong>1,284</strong><span class="muted">check-ins</span
              ><span class="accent">↗ 8.6%</span>
            </div>
            <svg
              class="chart"
              viewBox="0 0 660 195"
              role="img"
              aria-label="Attendance Monday to Sunday: 160, 182, 154, 211, 193, 226, 158 check-ins"
            >
              <g stroke="currentColor" opacity=".12">
                <path d="M35 20H640M35 65H640M35 110H640M35 155H640" />
              </g>
              <g fill="currentColor" opacity=".6" font-size="10">
                <text x="0" y="24">240</text>
                <text x="0" y="69">180</text>
                <text x="0" y="114">120</text>
                <text x="9" y="159">60</text>
              </g>
              <g v-for="(n, i) in [160, 182, 154, 211, 193, 226, 158]" :key="i">
                <rect
                  :x="55 + i * 84"
                  :y="200 - n * 0.75"
                  width="32"
                  :height="n * 0.75 - 45"
                  rx="5"
                  fill="var(--color-primary)"
                  :opacity="i === 5 ? 1 : 0.42"
                />
                <text
                  :x="71 + i * 84"
                  y="185"
                  text-anchor="middle"
                  fill="currentColor"
                  font-size="11"
                >
                  {{ ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'][i] }}
                </text>
              </g>
            </svg></AppCard
          >
          <AppCard class="panel"
            ><p class="eyebrow">TODAY'S FOCUS</p>
            <h2>Ready for a great week</h2>
            <p class="muted">A few things that need your attention.</p>
            <div class="focus-row">
              <Users :size="21" class="accent" />
              <div>
                <strong>5 new participants</strong>
                <p class="muted">Review pending registrations</p>
              </div>
            </div>
            <div class="focus-row">
              <CalendarDays :size="21" class="accent" />
              <div>
                <strong>Saturday is filling up</strong>
                <p class="muted">Performance group · 2 spaces</p>
              </div>
            </div>
            <div class="note">
              <ShieldCheck :size="21" />
              <div>
                <strong>Club operations, connected.</strong>
                <p>People, training and access in one shared workspace.</p>
              </div>
            </div></AppCard
          >
        </div>
        <div class="dashboard-grid">
          <AppCard class="panel"
            ><div class="panel-heading">
              <div>
                <p class="eyebrow">MONDAY, 19 OCTOBER</p>
                <h2>Upcoming training</h2>
              </div>
              <a :href="href('training')" class="text-link">Open editor ↗</a>
            </div>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>Session / group</th>
                    <th>Location</th>
                    <th>Booked</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="s in sessions" :key="s[0]">
                    <td>{{ s[0] }}</td>
                    <td>
                      <strong>{{ s[1] }}</strong
                      ><small>{{ s[2] }}</small>
                    </td>
                    <td>{{ s[3] }}</td>
                    <td>{{ s[4] }}</td>
                    <td>
                      <AppBadge variant="outline">{{ s[5] }}</AppBadge>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div></AppCard
          ><AppCard class="panel"
            ><p class="eyebrow">CLUB PULSE</p>
            <h2>Recent activity</h2>
            <div
              v-for="(item, i) in [
                'Junior skills lab published',
                'Jamie Park joined Community',
                'Attendance report ready',
              ]"
              :key="item"
              class="activity-row"
            >
              <span class="dot"></span>
              <div>
                <strong>{{ item }}</strong>
                <p class="muted">{{ [12, 28, 45][i] }} min ago · demo event</p>
              </div>
            </div>
            <p class="muted footnote">All activity is seeded for this presentation.</p></AppCard
          >
        </div>
      </template>
      <AppCard v-else-if="view === 'participants'" class="panel"
        ><div class="panel-heading">
          <div>
            <h2>Your club community</h2>
            <p class="muted">24 seeded participants across 3 groups</p>
          </div>
          <AppButton variant="outline" @click="preview('Invitation prepared')"
            >Invite participant</AppButton
          >
        </div>
        <div class="filters">
          <label
            >Search participants<AppInput
              v-model="query"
              placeholder="Search by name…"
              @update:model-value="page = 1" /></label
          ><label
            >Status<select aria-label="Status" v-model="status" @change="page = 1">
              <option>All</option>
              <option>Active</option>
              <option>Pending</option>
            </select></label
          ><AppBadge variant="outline">{{ filtered.length }} results</AppBadge>
        </div>
        <div class="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Participant</th>
                <th>Group</th>
                <th>Status</th>
                <th>Attendance</th>
                <th>Joined</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in rows" :key="p.id">
                <td>
                  <strong>{{ p.name }}</strong
                  ><small>{{ p.id }}</small>
                </td>
                <td>{{ p.group }}</td>
                <td>
                  <AppBadge variant="outline">{{ p.status }}</AppBadge>
                </td>
                <td>{{ p.attendance }}</td>
                <td>{{ p.joined }}</td>
                <td>
                  <AppButton
                    variant="ghost"
                    size="sm"
                    :aria-label="`View ${p.name}`"
                    @click="preview(p.name + ' selected')"
                    >View ↗</AppButton
                  >
                </td>
              </tr>
              <tr v-if="!rows.length">
                <td colspan="6">No matching participants.</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="pagination">
          <span class="muted"
            >Page {{ page }} of {{ Math.max(1, Math.ceil(filtered.length / 6)) }}</span
          ><AppButton variant="outline" :disabled="page === 1" @click="page--">Previous</AppButton
          ><AppButton variant="outline" :disabled="page * 6 >= filtered.length" @click="page++"
            >Next</AppButton
          >
        </div></AppCard
      >
      <div v-else-if="view === 'training'" class="editor-grid">
        <AppCard class="panel"
          ><p class="eyebrow">SESSION DETAILS</p>
          <h2>Make every session count</h2>
          <form @submit.prevent="preview('Training draft saved')">
            <label>Session title<AppInput v-model="title" required /></label
            ><label>Overview<textarea v-model="description" rows="4" /></label>
            <div class="two-columns">
              <label
                >Training group<select>
                  <option>Performance</option>
                  <option>Community</option>
                  <option>Junior academy</option>
                </select></label
              ><label>Coach<AppInput v-model="coach" /></label
              ><label>Date<AppInput type="date" v-model="date" /></label
              ><label>Start time<AppInput type="time" v-model="time" /></label
              ><label>Capacity<AppInput type="number" v-model="capacity" min="1" /></label
              ><label>Location<AppInput v-model="location" /></label>
            </div>
            <AppButton type="submit">Save local draft</AppButton>
          </form></AppCard
        >
        <div>
          <AppCard class="panel"
            ><p class="eyebrow">SESSION PREVIEW</p>
            <img
              class="session-image"
              src="/demo/sports-club/training.svg"
              alt="Local abstract illustration of a sports court"
            /><AppBadge variant="outline">Draft · synthetic</AppBadge>
            <h2 class="preview-title">{{ title }}</h2>
            <p class="muted">{{ description }}</p>
            <div class="focus-row">Monday · 07:00–08:00</div></AppCard
          >
          <p class="muted footnote">Edits stay in memory. No publication or notification occurs.</p>
        </div>
      </div>
      <div v-else-if="view === 'permissions'" class="editor-grid">
        <AppCard class="panel"
          ><p class="eyebrow">ACCESS DESIGN</p>
          <h2>Clear roles. Confident teams.</h2>
          <p class="muted">Illustrative permission choices; not a security evaluation.</p>
          <div class="role-tabs">
            <AppButton
              v-for="r in ['Administrator', 'Coach', 'Front desk']"
              :key="r"
              :variant="role === r ? 'primary' : 'outline'"
              @click="role = r"
              >{{ r }}</AppButton
            >
          </div>
          <h3>{{ role }} permissions</h3>
          <label
            v-for="(p, i) in [
              'View participants',
              'Edit participants',
              'View training',
              'Edit training',
              'Manage roles',
              'View queue',
            ]"
            :key="p"
            class="permission-row"
            ><span
              ><strong>{{ p }}</strong
              ><small>{{
                i < 2
                  ? 'People & membership'
                  : i < 4
                    ? 'Training operations'
                    : 'Workspace administration'
              }}</small></span
            ><input v-model="permissions" type="checkbox" :value="p" /></label></AppCard
        ><AppCard class="panel"
          ><ShieldCheck :size="32" class="accent" />
          <h2 class="preview-title">Permission preview</h2>
          <p class="muted">
            {{ permissions.length }} permissions selected for the
            {{ role.toLowerCase() }} presentation.
          </p>
          <h3>Administrator assignments</h3>
          <p class="muted">Fictional staff · in-memory role preview</p>
          <label v-for="admin in administrators" :key="admin.name" class="assignment-row"
            ><span>{{ admin.name }}</span
            ><select v-model="admin.role" :aria-label="`Role for ${admin.name}`">
              <option>Administrator</option>
              <option>Coach</option>
              <option>Front desk</option>
            </select></label
          >
          <div class="note">
            The real host owns authentication and RBAC. This fixture does not evaluate or persist
            role changes.
          </div></AppCard
        >
      </div>
      <AppCard v-else-if="view === 'queue'" class="panel"
        ><div class="panel-heading">
          <div>
            <p class="eyebrow">SYNTHETIC OPERATIONS</p>
            <h2>A calm view of background work</h2>
          </div>
          <AppBadge variant="outline">Snapshot · 09:00 UTC</AppBadge>
        </div>
        <div class="metrics queue-metrics">
          <div v-for="m in ['24 completed', '3 waiting', '1 needs review', '0 running']" :key="m">
            <strong>{{ m.split(' ')[0] }}</strong>
            <p class="muted">{{ m.slice(m.indexOf(' ') + 1) }}</p>
          </div>
        </div>
        <div class="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Job</th>
                <th>Queue</th>
                <th>Status</th>
                <th>Attempts</th>
                <th>Scheduled</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(job, i) in [
                  'Attendance summary',
                  'Membership reminder',
                  'Training digest',
                  'Export participants',
                ]"
                :key="job"
              >
                <td>
                  <strong>{{ job }}</strong
                  ><small>DEMO-{{ 401 + i }}</small>
                </td>
                <td>{{ i === 3 ? 'exports' : 'club-operations' }}</td>
                <td>
                  <AppBadge variant="outline">{{
                    ['Completed', 'Waiting', 'Needs review', 'Waiting'][i]
                  }}</AppBadge>
                </td>
                <td>{{ i === 2 ? '3 / 3' : '1 / 3' }}</td>
                <td>19 Oct · 09:00</td>
                <td>
                  <AppButton variant="ghost" size="sm" @click="preview(job + ' details opened')"
                    >Inspect ↗</AppButton
                  >
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="note">
          No workers, retries, emails or external integrations run in this fixture.
        </div></AppCard
      >
      <div v-else class="editor-grid">
        <AppCard class="panel"
          ><p class="eyebrow">YOUR WORKSPACE</p>
          <h2>Profile & preferences</h2>
          <form @submit.prevent="preview('Profile saved')">
            <div class="profile-summary">
              <span class="avatar">DA</span>
              <div>
                <strong>Demo administrator</strong>
                <p class="muted">Northline Sports Club · fictional account</p>
              </div>
            </div>
            <label>Display name<AppInput v-model="displayName" /></label
            ><label>Email<AppInput v-model="email" type="email" /></label>
            <div class="two-columns">
              <label
                >Timezone<select>
                  <option>UTC</option>
                </select></label
              ><label
                >Week starts on<select>
                  <option>Monday</option>
                  <option>Sunday</option>
                </select></label
              >
            </div>
            <AppButton type="submit">Save preferences locally</AppButton>
          </form></AppCard
        ><AppCard class="panel"
          ><p class="eyebrow">PRESENTATION OPTIONS</p>
          <h2>One system, different expressions</h2>
          <p class="muted">Existing theme extensions shape the shared shell and controls.</p>
          <div class="theme-links">
            <a
              v-for="p in ['default', 'compact', 'branded']"
              :key="p"
              :href="`/demo/settings?preset=${p}&mode=${mode}`"
              >{{ p }} ↗</a
            >
          </div>
          <a
            class="text-link"
            :href="`/demo/settings?preset=${preset}&mode=${mode === 'dark' ? 'light' : 'dark'}`"
            >Open {{ mode === 'dark' ? 'light' : 'dark' }} presentation ↗</a
          >
          <div class="note">
            For a live theme switch, use the shared theme button in the header.
          </div></AppCard
        >
      </div>
      <footer class="demo-footer">
        <span>GOADMIN / PRESENTATION EDITION</span
        ><span>Fictional club · local states · no backend mutations</span>
      </footer>
    </div></AdminLayout
  >
</template>
<style scoped>
.capability-cues,
.queue-indicator {
  display: flex;
  align-items: center;
  gap: 12px;
}
.queue-indicator {
  gap: 6px;
  color: var(--color-text-primary);
  letter-spacing: normal;
  font-size: 11px;
}
.notification-item {
  display: block;
  font-size: 12px;
  padding: 5px 0;
}
.notification-item strong {
  font-weight: 550;
}
.assignment-row {
  display: grid;
  grid-template-columns: 1fr 140px;
  gap: 10px;
  align-items: center;
  margin-top: 16px;
  font-size: 12px;
}
.assignment-row select {
  margin-top: 0;
}
.club-demo {
  font-size: 14px;
  line-height: 1.5;
}
.banner,
.heading,
.actions,
.panel-heading,
.metric-label,
.pagination,
.demo-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}
.banner {
  font-size: 10px;
  letter-spacing: 0.12em;
  color: var(--color-text-secondary);
  border-bottom: 1px solid var(--color-border-primary);
  padding-bottom: 16px;
}
.heading {
  margin: 24px 0;
}
.eyebrow {
  font-size: 10px;
  letter-spacing: 0.14em;
  font-weight: 700;
  color: var(--color-text-secondary);
  margin-bottom: 7px;
}
h1 {
  font-size: 30px;
  font-weight: 700;
  letter-spacing: -0.035em;
  line-height: 1.2;
  margin-bottom: 8px;
}
h2 {
  font-size: 17px;
  font-weight: 650;
  letter-spacing: -0.02em;
  margin-bottom: 5px;
}
h3 {
  font-weight: 650;
  margin: 24px 0 8px;
}
.muted {
  color: var(--color-text-secondary);
  font-size: 12px;
}
.metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 22px;
}
.metric {
  padding: 20px;
}
.metric-label {
  color: var(--color-text-secondary);
  font-size: 12px;
}
.metric strong,
.queue-metrics strong {
  display: block;
  font-size: 32px;
  letter-spacing: -0.04em;
  margin: 13px 0 8px;
  font-weight: 700;
}
.metric .muted {
  font-size: 11px;
}
.dashboard-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.85fr) minmax(0, 1fr);
  gap: 22px;
  margin-bottom: 22px;
}
.panel {
  padding: 22px;
  min-width: 0;
}
.panel-heading {
  margin-bottom: 18px;
  align-items: flex-start;
}
.chart-summary {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-top: 18px;
}
.chart-summary strong {
  font-size: 29px;
  font-weight: 650;
  letter-spacing: -0.03em;
}
.accent,
.text-link {
  color: var(--color-primary);
}
.chart {
  width: 100%;
  height: 190px;
  margin-top: 8px;
  color: var(--color-text-secondary);
}
.focus-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 0;
  border-bottom: 1px solid var(--color-border-primary);
  font-size: 12px;
}
.focus-row strong,
.activity-row strong {
  font-size: 12px;
  font-weight: 600;
}
.note {
  display: flex;
  gap: 10px;
  background: var(--color-surface-variant);
  padding: 16px;
  border-radius: 8px;
  margin-top: 19px;
  font-size: 12px;
  line-height: 1.65;
  color: var(--color-text-secondary);
}
.note strong {
  color: var(--color-text-primary);
}
.table-scroll {
  overflow-x: auto;
}
table {
  width: 100%;
  border-collapse: collapse;
  white-space: nowrap;
  text-align: left;
  font-size: 12px;
}
th {
  font-size: 10px;
  letter-spacing: 0.035em;
  font-weight: 600;
  color: var(--color-text-secondary);
  padding: 10px 12px;
  background: var(--color-surface-variant);
}
td {
  padding: 14px 12px;
  border-bottom: 1px solid var(--color-border-primary);
}
td strong {
  font-weight: 600;
}
small {
  display: block;
  font-size: 10px;
  color: var(--color-text-secondary);
  margin-top: 3px;
}
.text-link {
  font-size: 12px;
  font-weight: 600;
}
.activity-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 16px 0;
  border-bottom: 1px solid var(--color-border-primary);
}
.dot {
  display: block;
  background: var(--color-primary);
  border-radius: 50%;
  margin-top: 7px;
  flex-shrink: 0;
  width: 6px;
  height: 6px;
}
.footnote {
  margin-top: 18px;
  font-size: 11px;
}
.demo-footer {
  border-top: 1px solid var(--color-border-primary);
  padding: 18px 0 4px;
  font-size: 9px;
  letter-spacing: 0.07em;
  color: var(--color-text-secondary);
}
.filters {
  display: flex;
  align-items: flex-end;
  gap: 16px;
  margin: 24px 0;
}
.filters label:first-child {
  flex: 1;
}
.filters label:nth-child(2) {
  min-width: 160px;
}
label {
  display: block;
  font-size: 12px;
  font-weight: 550;
}
label > input,
label > select,
label > textarea {
  margin-top: 8px;
}
select,
textarea {
  background: var(--color-card);
  border: 1px solid var(--color-border-primary);
  border-radius: 6px;
  padding: 8px 10px;
  width: 100%;
  color: var(--color-text-primary);
  font: inherit;
}
select:focus-visible,
textarea:focus-visible,
input[type='checkbox']:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 3px;
}
select {
  min-height: var(--goadmin-control-height, 36px);
}
.pagination {
  justify-content: flex-end;
  margin-top: 20px;
}
.pagination > span {
  margin-right: auto;
}
.editor-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.65fr) minmax(0, 1fr);
  gap: 22px;
  margin-bottom: 24px;
}
.editor-grid > .panel {
  align-self: start;
}
form {
  display: grid;
  gap: 22px;
  margin-top: 24px;
}
form > button {
  justify-self: start;
}
.two-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.session-image {
  width: 100%;
  border-radius: 8px;
  margin: 18px 0;
}
.preview-title {
  margin-top: 16px;
}
.role-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 26px;
}
.permission-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 17px 0;
  border-bottom: 1px solid var(--color-border-primary);
}
.permission-row input {
  width: 18px;
  height: 18px;
  accent-color: var(--color-primary);
}
.queue-metrics {
  margin: 26px 0;
  background: var(--color-surface-variant);
  padding: 12px 22px;
  border-radius: 8px;
}
.profile-summary {
  display: flex;
  align-items: center;
  gap: 14px;
}
.avatar {
  display: grid;
  place-items: center;
  border-radius: 50%;
  width: 54px;
  height: 54px;
  background: var(--color-primary);
  color: var(--color-primary-contrast);
  font-weight: 650;
}
.theme-links {
  display: grid;
  gap: 10px;
  margin: 24px 0;
}
.theme-links a {
  padding: 14px;
  background: var(--color-surface-variant);
  border-radius: 8px;
  text-transform: capitalize;
}
.notice {
  padding: 12px 16px;
  background: var(--color-surface-variant);
  border-left: 3px solid var(--color-primary);
  margin-bottom: 20px;
  font-size: 12px;
}
@media (max-width: 1199px) {
  .dashboard-grid {
    grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
  }
  .panel {
    padding: 18px;
  }
  .metric {
    padding: 16px;
  }
  .actions {
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}
@media (max-width: 767px) {
  .capability-cues {
    width: 100%;
    justify-content: flex-end;
  }
  .seed-label {
    margin-right: auto;
    font-size: 9px;
  }
  .assignment-row {
    grid-template-columns: 1fr 130px;
  }
  .banner {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }
  .heading {
    display: block;
  }
  h1 {
    font-size: 28px;
  }
  .actions {
    justify-content: flex-start;
    margin-top: 16px;
  }
  .metrics {
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .metric strong {
    font-size: 27px;
  }
  .dashboard-grid,
  .editor-grid {
    grid-template-columns: 1fr;
  }
  .two-columns {
    grid-template-columns: 1fr;
  }
  .filters {
    flex-wrap: wrap;
  }
  .filters label:first-child {
    flex-basis: 100%;
  }
  .demo-footer {
    align-items: flex-start;
    flex-direction: column;
    gap: 5px;
  }
  .chart {
    height: 160px;
  }
  .panel-heading {
    flex-wrap: wrap;
  }
  .metric-label {
    font-size: 11px;
  }
  .metric-label svg {
    width: 15px;
  }
  .pagination {
    gap: 8px;
  }
  .pagination > span {
    font-size: 10px;
  }
}
</style>
