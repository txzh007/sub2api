<template>
  <div class="tt-batch bt-account-batch">
    <div class="bt-account-controls">
      <div class="bt-field">
        <span>{{ t('admin.batchTest.kind') }}</span>
        <div class="bt-segments" role="group" :aria-label="t('admin.batchTest.kind')">
          <button :aria-pressed="kind === 'text'" :disabled="busy" @click="kind = 'text'"><Icon name="terminal" size="sm" />{{ t('admin.batchTest.text') }}</button>
          <button v-if="hasImages" :aria-pressed="kind === 'image'" :disabled="busy" @click="kind = 'image'"><Icon name="grid" size="sm" />{{ t('admin.batchTest.image') }}</button>
          <button v-if="account.platform === 'grok'" :aria-pressed="kind === 'video'" :disabled="busy" @click="kind = 'video'"><Icon name="play" size="sm" />{{ t('admin.batchTest.video') }}</button>
        </div>
      </div>
      <label class="bt-field">
        <span>{{ t('admin.batchTest.concurrency') }}</span>
        <select v-model.number="concurrency" class="bt-input" :disabled="busy">
          <option v-for="n in 3" :key="n" :value="n">{{ n === 1 ? t('admin.batchTest.design.serial') : t('admin.batchTest.design.parallel', { count: n }) }}</option>
        </select>
      </label>
      <label v-if="account.platform === 'openai' && kind === 'text'" class="bt-field">
        <span>{{ t('admin.accounts.openai.testMode') }}</span>
        <select v-model="mode" class="bt-input" :disabled="busy"><option value="default">{{ t('admin.accounts.openai.testModeDefault') }}</option><option value="compact">Compact</option></select>
      </label>
    </div>
    <textarea v-if="kind !== 'text'" v-model="prompt" class="bt-input bt-account-prompt" :placeholder="t('admin.batchTest.prompt')" :aria-label="t('admin.batchTest.prompt')" :disabled="busy" maxlength="16000" />

    <section class="bt-account-models">
      <div class="bt-row bt-between bt-account-section-head">
        <div class="bt-row"><h3 class="bt-heading">{{ t('admin.pelicanTest.chooseModel') }}</h3><span class="bt-count">{{ selected.length }}</span></div>
        <button v-if="rows.length" class="bt-link bt-row" @click="showModels = !showModels">{{ showModels ? t('admin.batchTest.design.collapse') : t('admin.batchTest.design.expandModels') }}<Icon :name="showModels ? 'chevronUp' : 'chevronDown'" size="xs" /></button>
      </div>
      <BatchModelPicker v-show="showModels" :key="kind" v-model:selected="selected" :models="eligible.map(m => ({ id: m.id, label: m.display_name }))" :loading="loading" :disabled="busy" />
      <div v-if="!showModels" class="bt-row bt-wrap"><span v-for="model in selected.slice(0, 3)" :key="model" class="bt-badge bt-mono">{{ model }}</span><span v-if="selected.length > 3" class="bt-badge">+{{ selected.length - 3 }}</span></div>
    </section>

    <section v-if="rows.length" class="bt-account-results">
      <div class="bt-row bt-between bt-wrap">
        <div class="bt-row"><h3 class="bt-heading">{{ t('admin.pelicanTest.results') }}</h3><span class="bt-muted bt-small bt-mono">{{ finished }} / {{ rows.length }}</span></div>
        <label class="bt-row bt-muted bt-small"><input v-model="onlyFailed" type="checkbox" />{{ t('admin.batchTest.onlyFailed') }}</label>
      </div>
      <div class="bt-progress" role="progressbar" :aria-valuenow="finished" :aria-valuemin="0" :aria-valuemax="rows.length" :aria-label="t('admin.pelicanTest.results')"><span :style="{ width: `${finished / rows.length * 100}%` }" /></div>
      <div class="bt-account-result-list" aria-live="polite">
        <details v-for="row in visibleRows" :key="row.model" class="bt-account-result">
          <summary class="bt-row">
            <Icon name="chevronRight" size="xs" class="bt-result-chevron bt-muted" />
            <strong class="bt-spacer bt-result-model" :title="row.model">{{ row.model }}</strong>
            <span class="bt-mono bt-muted bt-small">{{ row.duration ? `${(row.duration / 1000).toFixed(1)}s` : '—' }}</span>
            <span class="bt-status" :data-status="row.status">{{ t(`admin.batchTest.status.${row.status}`) }}</span>
          </summary>
          <div class="bt-account-output">
            <p v-if="row.actualModel" class="bt-muted bt-small bt-mono">{{ row.actualModel }}</p>
            <p v-if="row.error" class="bt-danger bt-small">{{ row.error }}</p>
            <pre v-if="row.text" class="bt-mono">{{ row.text }}</pre>
            <img v-for="url in row.images" :key="url" :src="url" :alt="row.model" />
            <video v-for="url in row.videos" :key="url" :src="url" controls />
            <button v-if="!busy" class="bt-button" @click="launch([row.model], true)"><Icon name="refresh" size="xs" />{{ t('admin.batchTest.retry') }}</button>
          </div>
        </details>
        <p v-if="!visibleRows.length" class="bt-muted bt-small bt-no-failures">{{ t('admin.pelicanTest.noFailures') }}</p>
      </div>
    </section>
    <div v-else-if="kind === 'text'" class="bt-notice"><Icon name="infoCircle" size="sm" /><span>{{ t('admin.batchTest.textHint') }}</span></div>

    <Teleport :to="actionsTarget || 'body'" :disabled="!actionsTarget">
    <footer class="tt-batch bt-account-actions">
      <span v-if="rows.length" class="bt-muted bt-small">{{ t('admin.batchTest.progress', { done: finished, total: rows.length, success: succeeded, failed: failed.length }) }}</span>
      <span v-else class="bt-muted bt-small">{{ t('admin.batchTest.design.selectHint') }}</span>
      <div class="bt-row bt-wrap">
        <button v-if="rows.length" class="bt-button" @click="copyResults"><Icon name="copy" size="sm" />{{ t('admin.batchTest.copy') }}</button>
        <button v-if="failed.length && !busy" class="bt-button" @click="launch(failed.map(r => r.model), true)">{{ t('admin.batchTest.retryFailed') }}</button>
        <button v-if="busy" class="bt-button" @click="stop">{{ t('admin.batchTest.stop') }}</button>
        <button v-else class="bt-button bt-primary" :disabled="!selected.length || loading" @click="launch(selected)"><Icon name="play" size="sm" />{{ t('admin.batchTest.start', { count: selected.length }) }}</button>
      </div>
    </footer>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import BatchModelPicker from '../BatchModelPicker.vue'
import '../batch-test.css'
import { useI18n } from 'vue-i18n'
import type { Account, ClaudeModel } from '@/types'
import { streamAccountTest } from '@/api/ttoken/accountTestStream'
import { useClipboard } from '@/composables/useClipboard'
const props = defineProps<{ account: Account; models: ClaudeModel[]; loading: boolean; actionsTarget?: HTMLElement | null }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
type State = 'queued' | 'running' | 'success' | 'error' | 'cancelled'
type Row = { model: string; status: State; duration: number; text: string; error: string; actualModel: string; images: string[]; videos: string[] }
const showModels = ref(true)
const prompt = ref(''), kind = ref('text'), mode = ref('default')
const concurrency = ref(1), selected = ref<string[]>([]), busy = ref(false), onlyFailed = ref(false), rows = ref<Row[]>([])
const controllers = new Map<string, AbortController>()
let generation = 0
let lastConfig = { prompt: '', kind: 'text', mode: 'default', accountID: props.account.id }
const modelKind = (id: string) => /video|seedance|sora/i.test(id) ? 'video' : /image|imagine/i.test(id) ? 'image' : 'text'
const hasImages = computed(() => props.models.some(m => modelKind(m.id) === 'image'))
const eligible = computed(() => props.models.filter(m => modelKind(m.id) === kind.value))
const failed = computed(() => rows.value.filter(r => r.status === 'error'))
const succeeded = computed(() => rows.value.filter(r => r.status === 'success').length)
const finished = computed(() => rows.value.filter(r => !['queued', 'running'].includes(r.status)).length)
const visibleRows = computed(() => onlyFailed.value ? failed.value : rows.value)
watch([kind, () => props.account.id], () => { stop(); selected.value = []; rows.value = []; prompt.value = kind.value === 'text' ? '' : t('admin.accounts.imagePromptDefault') })
watch(busy, value => { if (value) showModels.value = false })
watch(() => rows.value.length, value => { if (!value) showModels.value = true })
watch(eligible, value => { selected.value = selected.value.filter(id => value.some(m => m.id === id)) })
function stop() {
  generation++
  for (const controller of controllers.values()) controller.abort()
  controllers.clear()
  for (const row of rows.value) if (['queued', 'running'].includes(row.status)) row.status = 'cancelled'
  busy.value = false
}
async function launch(ids: string[], retry = false) {
  if (busy.value || !ids.length) return
  const token = ++generation
  busy.value = true
  onlyFailed.value = false
  if (!retry) {
    lastConfig = { prompt: prompt.value.trim(), kind: kind.value, mode: mode.value, accountID: props.account.id }
    rows.value = [...new Set(ids)].map(model => ({ model, status: 'queued', duration: 0, text: '', error: '', actualModel: '', images: [], videos: [] }))
  }
  const queue = rows.value.filter(row => ids.includes(row.model))
  for (const row of queue) Object.assign(row, { status: 'queued', duration: 0, text: '', error: '', actualModel: '', images: [], videos: [] })
  const config = { ...lastConfig }
  const platform = props.account.platform
  const worker = async () => {
    while (queue.length && generation === token) {
      const row = queue.shift()!
      row.status = 'running'
      const started = performance.now(), controller = new AbortController()
      controllers.set(row.model, controller)
      let timedOut = false
      const timer = setTimeout(() => { timedOut = true; controller.abort() }, config.kind === 'text' ? 120000 : 240000)
      try {
        await streamAccountTest(config.accountID, {
          model_id: row.model, prompt: config.prompt,
          mode: platform === 'grok' ? config.kind : platform === 'openai' ? (config.kind === 'text' ? config.mode : 'default') : undefined
        }, controller.signal, event => {
          if (generation !== token) return
          if (event.model) row.actualModel = event.model
          if (event.text) row.text = (row.text + event.text).slice(-32000)
          if (event.image_url && row.images.length < 8) row.images.push(event.image_url)
          if (event.video_url && row.videos.length < 4) row.videos.push(event.video_url)
          if (event.type === 'error' || (event.type === 'test_complete' && !event.success)) { row.status = 'error'; row.error = event.error || t('admin.accounts.testFailed') }
          else if (event.type === 'test_complete') row.status = 'success'
        })
      } catch (error) {
        if (generation === token) { row.status = 'error'; row.error = timedOut ? t('admin.batchTest.timeout') : error instanceof Error ? error.message : t('common.unknownError') }
      } finally {
        clearTimeout(timer)
        if (generation === token) { row.duration = performance.now() - started; controllers.delete(row.model) }
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency.value, queue.length) }, worker))
  if (generation === token) busy.value = false
}
function copyResults() { copyToClipboard(rows.value.map(r => `${r.model}\t${t(`admin.batchTest.status.${r.status}`)}\t${(r.duration / 1000).toFixed(1)}s\t${r.error || r.text}`).join('\n'), t('admin.accounts.outputCopied')) }
onBeforeUnmount(stop)
</script>

<style scoped>
.bt-account-batch { display: flex; flex-direction: column; gap: 22px; }
.bt-account-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 14px; }
.bt-account-controls > div { flex: 1; min-width: 240px; }
.bt-account-controls > label { flex: 0 1 155px; }
.bt-account-models { display: flex; flex-direction: column; gap: 14px; }
.bt-account-models :deep(.bt-model-list) { max-height: 240px; }
.bt-account-section-head { min-height: 24px; }
.bt-account-prompt { min-height: 88px; resize: vertical; }
.bt-account-results { display: flex; flex-direction: column; gap: 13px; }
.bt-account-result-list { max-height: 280px; overflow-y: auto; border: 1px solid var(--bt-line); border-radius: 9px; }
.bt-account-result + .bt-account-result { border-top: 1px solid var(--bt-line); }
.bt-account-result summary { padding: 13px 14px; list-style: none; cursor: pointer; }
.bt-account-result summary::-webkit-details-marker { display: none; }
.bt-account-result summary:hover { background: var(--bt-soft); }
.bt-account-result[open] .bt-result-chevron { transform: rotate(90deg); }
.bt-result-model { font-size: 12px; font-weight: 550; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bt-account-output { display: flex; flex-direction: column; align-items: start; gap: 12px; border-top: 1px solid var(--bt-line); padding: 16px; background: var(--bt-soft); }
.bt-account-output pre { max-height: 210px; overflow: auto; width: 100%; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; line-height: 1.8; }
.bt-account-output :is(img, video) { max-height: 240px; max-width: 100%; object-fit: contain; border-radius: 8px; }
.bt-account-output p { overflow-wrap: anywhere; }
.bt-account-actions { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; width: 100%; }
.bt-no-failures { padding: 20px; text-align: center; }
@media (max-width: 639px) {
  .bt-account-controls > div { flex-basis: 100%; }
  .bt-account-controls > label { flex: 1; }
  .bt-account-actions > div { width: 100%; justify-content: flex-end; }
}
</style>
