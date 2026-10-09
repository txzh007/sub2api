<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import adminAPI from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { Account, AccountListItem } from '@/types'
import { getAccountActivity, activityLatency, activityRate, type ActivityResponse } from '@/api/ttoken/accountActivity'
import '../batch-test.css'

const props = defineProps<{ account: AccountListItem | null; accounts: AccountListItem[]; hours: 1 | 24 }>()
const emit = defineEmits<{
  close: []; updated: [account: Account]; select: [account: AccountListItem]; 'update:hours': [hours: 1 | 24]; stats: [account: Account]
}>()
const { t, locale } = useI18n()
const app = useAppStore()
const current = ref<Account | null>(null)
const data = ref<ActivityResponse | null>(null)
const loading = ref(false)
const failed = ref(false)
const saving = ref(false)
const priority = ref<number | string>(0)
const errorsOnly = ref(false)
const summary = computed(() => data.value?.accounts[String(props.account?.id)])
const records = computed(() => (data.value?.recent || []).filter(r => !errorsOnly.value || r.kind !== 'success'))
const maxBucket = computed(() => Math.max(1, ...summary.value?.trend.map(b => b.success + b.failed + b.recovered) || []))
const restricted = computed(() => [current.value?.rate_limit_reset_at, current.value?.overload_until, current.value?.temp_unschedulable_until].some(v => v && new Date(v).getTime() > Date.now()))
const selectOptions = computed(() => {
  const options = [...props.accounts]
  if (props.account && !options.some(a => a.id === props.account!.id)) options.unshift(props.account)
  return options
})
let controller: AbortController | undefined
let generation = 0
let timer: ReturnType<typeof setInterval> | undefined

function time(value: string, date = false) {
  return new Date(value).toLocaleString(locale.value, date ? { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' } : { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
function bucketLabel(index: number) {
  if (!data.value || !summary.value) return ''
  const start = new Date(data.value.start_time).getTime()
  const end = new Date(data.value.end_time).getTime()
  return t('admin.accountActivity.bucket', { time: time(new Date(start + (end - start) * index / 12).toISOString(), props.hours === 24), ...summary.value.trend[index] })
}
async function refresh(loadAccount = false) {
  const id = props.account?.id
  if (!id) return
  const token = ++generation
  controller?.abort(); controller = new AbortController()
  loading.value = true; failed.value = false
  try {
    const [snapshot, account] = await Promise.all([
      getAccountActivity([id], props.hours, true, controller.signal),
      loadAccount ? adminAPI.accounts.getById(id) : Promise.resolve(null)
    ])
    if (token !== generation) return
    data.value = snapshot
    if (account) { current.value = account; priority.value = account.priority }
  } catch {
    if (token === generation) failed.value = true
  } finally { if (token === generation) loading.value = false }
}
watch([() => props.account?.id, () => props.hours], ([id], [previousId]) => {
  generation++; controller?.abort(); clearInterval(timer)
  data.value = null; failed.value = false; loading.value = false
  if (id !== previousId) { current.value = null; errorsOnly.value = false }
  if (!id) return
  void refresh(!current.value)
  timer = setInterval(() => {
    if (!loading.value && !saving.value && document.visibilityState === 'visible') void refresh()
  }, 30_000)
}, { immediate: true })

async function save(kind: 'priority' | 'schedule') {
  if (!current.value || saving.value) return
  const id = current.value.id
  const value = Number(priority.value)
  if (kind === 'priority' && (priority.value === '' || !Number.isInteger(value) || value < 1 || value > 9999)) {
    app.showError(t('admin.accountActivity.invalidPriority')); return
  }
  saving.value = true
  try {
    const updated = kind === 'priority'
      ? await adminAPI.accounts.update(id, { priority: value })
      : await adminAPI.accounts.setSchedulable(id, !current.value.schedulable)
    emit('updated', updated)
    if (props.account?.id === id) { current.value = updated; priority.value = updated.priority }
    app.showSuccess(t('admin.accountActivity.changed'))
  } catch { app.showError(t('admin.accountActivity.changeFailed')) }
  finally { saving.value = false }
}
function selectAccount(event: Event) {
  const id = Number((event.target as HTMLSelectElement).value)
  const account = selectOptions.value.find(a => a.id === id)
  if (account) emit('select', account)
}
onUnmounted(() => { generation++; controller?.abort(); clearInterval(timer) })
</script>

<template>
  <BaseDialog :show="!!account" :title="t('admin.accountActivity.title')" width="extra-wide" @close="emit('close')">
    <div class="tt-batch aa-detail">
      <header class="aa-heading">
        <div class="aa-identity"><span class="aa-avatar"><Icon name="chartBar" size="md" /></span><div><h2>{{ account?.name }}</h2><p>#{{ account?.id }} · {{ account?.platform }} · {{ account?.type }}</p></div></div>
        <select class="bt-input aa-account-select" :aria-label="t('admin.accountActivity.selectAccount')" :value="account?.id" :disabled="saving" @change="selectAccount"><option v-for="a in selectOptions" :key="a.id" :value="a.id">{{ a.name }}</option></select>
      </header>
      <div class="aa-controls">
        <div class="bt-segments"><button :aria-pressed="hours === 1" @click="emit('update:hours', 1)">{{ t('admin.accountActivity.hour') }}</button><button :aria-pressed="hours === 24" @click="emit('update:hours', 24)">{{ t('admin.accountActivity.day') }}</button></div>
        <button class="bt-button" :disabled="loading" @click="refresh(!current)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('admin.accountActivity.refresh') }}</button>
      </div>
      <div v-if="failed" class="bt-notice" role="alert">{{ t(data ? 'admin.accountActivity.stale' : 'admin.accountActivity.loadFailed') }}</div>
      <div v-if="data && !data.monitoring_enabled" class="bt-notice">{{ t('admin.accountActivity.monitoringOff') }}</div>
      <div v-if="loading && !data" class="aa-loading">{{ t('common.loading') }}</div>
      <template v-if="summary && data">
        <section class="aa-metrics">
          <div><span>{{ t('admin.accountActivity.requests') }}</span><strong>{{ (summary.success + summary.failed).toLocaleString() }}</strong><small>{{ t('admin.accountActivity.counts', { success: summary.success, failed: summary.failed }) }}</small></div>
          <div><span>{{ t('admin.accountActivity.rate') }}</span><strong :class="{ 'aa-error': summary.failed > 0 }">{{ activityRate(summary, data.monitoring_enabled) }}</strong><small>{{ t('admin.accountActivity.recovered') }} <b>{{ summary.recovered }}</b></small></div>
          <div><span>{{ t('admin.accountActivity.latency') }}</span><strong>{{ activityLatency(summary.avg_first_token_ms) }}</strong><small>{{ t('admin.accountActivity.duration') }} {{ activityLatency(summary.avg_duration_ms) }}</small></div>
          <div><span>{{ t('admin.accountActivity.cost') }}</span><strong>${{ summary.cost.toFixed(4) }}</strong><small>{{ t('admin.accountActivity.limited') }} <b>{{ summary.rate_limited }}</b></small></div>
        </section>
        <section class="aa-timeline">
          <div class="aa-section-title"><h3>{{ t('admin.accountActivity.trend') }}</h3><div class="aa-legend"><span class="aa-dot success">{{ t('admin.accountActivity.success') }}</span><span class="aa-dot failed">{{ t('admin.accountActivity.failed') }}</span><span class="aa-dot recovered">{{ t('admin.accountActivity.recoveredLegend') }}</span></div></div>
          <div class="aa-bars" role="img" :aria-label="t('admin.accountActivity.trend')">
            <div v-for="(bucket, index) in summary.trend" :key="index" class="aa-bar" :title="bucketLabel(index)" :aria-label="bucketLabel(index)" tabindex="0">
              <i class="aa-bar-success" :style="{ height: `${bucket.success / maxBucket * 100}%` }"></i><i class="aa-bar-failed" :style="{ height: `${bucket.failed / maxBucket * 100}%` }"></i><i class="aa-bar-recovered" :style="{ height: `${bucket.recovered / maxBucket * 100}%` }"></i>
            </div>
          </div>
          <div class="aa-axis"><span>{{ time(data.start_time, hours === 24) }}</span><span>{{ t('admin.accountActivity.updated', { time: time(data.end_time) }) }}</span></div>
        </section>
        <section class="aa-records">
          <div class="aa-section-title"><h3>{{ t('admin.accountActivity.recent') }}</h3><label class="aa-filter"><input v-model="errorsOnly" type="checkbox" />{{ t('admin.accountActivity.errorsOnly') }}</label></div>
          <div class="aa-request-scroll">
            <table v-if="records.length"><thead><tr><th>{{ t('admin.accountActivity.model') }}</th><th>{{ t('admin.accountActivity.outcome') }}</th><th>{{ t('admin.accountActivity.timing') }}</th><th>{{ t('admin.accountActivity.amount') }}</th></tr></thead><tbody>
              <template v-for="record in records" :key="record.id"><tr>
                <td><strong>{{ record.model || '—' }}</strong><small>{{ time(record.created_at, true) }}</small></td>
                <td><span class="aa-result" :data-kind="record.kind">{{ t(`admin.accountActivity.${record.kind === 'recovered' ? 'retryRecovered' : record.kind}`) }}<template v-if="record.status_code"> · {{ record.status_code }}</template></span></td>
                <td class="aa-mono">{{ activityLatency(record.first_token_ms) }} / {{ activityLatency(record.duration_ms) }}</td>
                <td class="aa-mono">{{ record.kind === 'success' ? `$${record.cost.toFixed(4)}` : '—' }}</td>
              </tr><tr v-if="record.message"><td colspan="4" class="aa-message"><details><summary>{{ t('admin.accountActivity.detail') }}</summary><p>{{ record.message }}</p></details></td></tr></template>
            </tbody></table>
            <div v-else class="aa-empty"><Icon name="chartBar" size="lg" /><p>{{ t(errorsOnly ? 'admin.accountActivity.noErrors' : 'admin.accountActivity.noRequests') }}</p></div>
          </div>
        </section>
        <p class="aa-footnote">{{ t('admin.accountActivity.definition') }} {{ t('admin.accountActivity.coverage') }}</p>
      </template>
    </div>
    <template #footer>
      <div class="tt-batch aa-footer">
        <div class="aa-actions"><span class="aa-scheduling" :class="{ 'is-active': current?.schedulable }">{{ current ? t(current.schedulable ? 'admin.accountActivity.active' : 'admin.accountActivity.paused') : '—' }}</span>
          <button class="bt-button" :disabled="!current || saving" @click="save('schedule')">{{ t(current?.schedulable ? 'admin.accountActivity.pause' : 'admin.accountActivity.resume') }}</button>
          <div class="aa-priority"><label for="tt-activity-priority">{{ t('admin.accountActivity.priority') }}</label><input id="tt-activity-priority" v-model="priority" class="bt-input" type="number" min="1" max="9999" step="1" :disabled="!current || saving" @keydown.enter="save('priority')" /><button class="bt-button bt-primary" :disabled="!current || saving || Number(priority) === current.priority" @click="save('priority')">{{ t('admin.accountActivity.save') }}</button></div>
          <button class="bt-button aa-stats" :disabled="!current || saving" @click="current && emit('stats', current)">{{ t('admin.accountActivity.stats') }}<Icon name="chevronRight" size="xs" /></button>
        </div>
        <p v-if="current && current.status !== 'active'" class="aa-warning">{{ t('admin.accountActivity.accountUnavailable', { status: current.status }) }}</p>
        <p v-if="restricted" class="aa-warning">{{ t('admin.accountActivity.restricted') }}</p>
        <p class="aa-footnote">{{ t('admin.accountActivity.priorityHint') }}</p>
      </div>
    </template>
  </BaseDialog>
</template>

<style scoped>
.aa-detail { display: flex; flex-direction: column; gap: 18px; }
.aa-heading,.aa-identity,.aa-controls,.aa-section-title,.aa-legend,.aa-actions,.aa-priority { display: flex; align-items: center; }
.aa-heading,.aa-controls,.aa-section-title { justify-content: space-between; gap: 12px; }
.aa-identity { gap: 12px; min-width: 0; }
.aa-avatar { display: grid; place-items: center; flex-shrink: 0; width: 42px; height: 42px; border: 1px solid var(--bt-selected); border-radius: 12px; background: var(--bt-tint); color: var(--bt-accent); }
.aa-heading h2 { font-size: 17px; font-weight: 650; overflow-wrap: anywhere; }
.aa-heading p { margin-top: 3px; color: var(--bt-muted); font: 11px ui-monospace,monospace; }
.aa-account-select { max-width: 200px; width: 35%; font-size: 12px; }
.aa-metrics { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 1px; overflow: hidden; border: 1px solid var(--bt-line); border-radius: 12px; background: var(--bt-line); }
.aa-metrics > div { min-width: 0; background: var(--bt-soft); padding: 16px; display: flex; flex-direction: column; gap: 8px; }
.aa-metrics span,.aa-metrics small { color: var(--bt-muted); font-size: 11px; }
.aa-metrics strong { font: 600 clamp(18px,2.1vw,25px) ui-monospace,monospace; letter-spacing: -.06em; white-space: nowrap; }
.aa-metrics strong.aa-error { color: #c76850; }
.aa-metrics small { font-size: 10px; }
.aa-section-title h3 { font-size: 12px; font-weight: 650; }
.aa-legend { gap: 12px; font-size: 10px; color: var(--bt-muted); }
.aa-dot { display: flex; gap: 5px; align-items: center; }
.aa-dot::before { content: ''; width: 6px; height: 6px; border-radius: 2px; background: var(--bt-accent); }
.aa-dot.failed::before,.aa-bar-failed { background: #cf7a63; }
.aa-dot.recovered::before,.aa-bar-recovered { background: #d3ab63; }
.aa-bars { display: flex; gap: 7px; height: 63px; margin-top: 15px; }
.aa-bar { flex: 1; display: flex; flex-direction: column-reverse; height: 100%; border-radius: 3px; overflow: hidden; background: var(--bt-soft); outline-color: var(--bt-accent); }
.aa-bar i { display: block; width: 100%; }
.aa-bar-success { background: var(--bt-accent); opacity: .7; }
.aa-axis { display: flex; justify-content: space-between; margin-top: 7px; color: var(--bt-muted); font-size: 10px; }
.aa-filter { display: flex; align-items: center; gap: 6px; font-size: 11px; color: var(--bt-muted); cursor: pointer; }
.aa-request-scroll { margin-top: 10px; max-height: 290px; overflow: auto; border: 1px solid var(--bt-line); border-radius: 10px; }
table { width: 100%; text-align: left; font-size: 11px; border-collapse: collapse; }
th { position: sticky; top: 0; background: var(--bt-soft); color: var(--bt-muted); font-size: 10px; font-weight: 500; z-index: 1; white-space: nowrap; }
th,td { padding: 11px 13px; border-bottom: 1px solid var(--bt-line); }
td strong { display: block; max-width: 290px; overflow-wrap: anywhere; font-weight: 550; }
td small { display: block; color: var(--bt-muted); margin-top: 4px; font-size: 10px; }
.aa-mono { font: 10px ui-monospace,monospace; white-space: nowrap; }
.aa-result { display: inline-flex; padding: 3px 6px; border-radius: 5px; background: var(--bt-tint); color: var(--bt-accent); white-space: nowrap; }
.aa-result[data-kind='failed'] { color: #c76850; background: #cf7a6312; }
.aa-result[data-kind='recovered'] { color: #a98742; background: #d3ab6312; }
td.aa-message { padding-top: 6px; padding-bottom: 6px; color: var(--bt-muted); font-size: 10px; }
.aa-message summary { cursor: pointer; }
.aa-message p { padding: 8px 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.aa-footnote { font-size: 10px; line-height: 1.7; color: var(--bt-muted); }
.aa-empty { padding: 25px; text-align: center; color: var(--bt-muted); font-size: 12px; }
.aa-empty svg { margin: 0 auto 10px; opacity: .5; }
.aa-loading { padding: 65px 20px; text-align: center; color: var(--bt-muted); }
.aa-footer { width: 100%; }
.aa-footer .aa-footnote { margin-top: 10px; }
.aa-actions { flex-wrap: wrap; gap: 10px; }
.aa-scheduling { color: var(--bt-muted); font-size: 11px; }
.aa-scheduling.is-active { color: var(--bt-accent); }
.aa-priority { gap: 8px; margin-left: 8px; font-size: 11px; }
.aa-priority input { width: 68px; padding: 6px 9px; min-height: 32px; }
.aa-stats { margin-left: auto; }
.aa-warning { font-size: 11px; color: #bd8641; margin-top: 8px; }
@media (max-height: 820px) {
  .aa-detail { gap: 12px; }
  .aa-avatar { width: 36px; height: 36px; }
  .aa-metrics > div { padding: 12px 14px; gap: 6px; }
  .aa-metrics strong { font-size: 22px; }
  .aa-bars { height: 38px; margin-top: 10px; }
  .aa-request-scroll { max-height: 160px; }
}
@media (max-width: 640px) { .aa-metrics { grid-template-columns: repeat(2,minmax(0,1fr)); }.aa-metrics>div { padding: 12px; }.aa-account-select { max-width: 140px; }.aa-priority { margin-left: 0; }.aa-stats { margin-left: 0; }.aa-heading h2 { font-size: 15px; } }
</style>
