<template>
  <div class="tt-batch bt-workbench">
    <header class="bt-page-head">
      <div class="bt-row">
        <span class="bt-page-icon"><Icon name="grid" size="lg" /></span>
        <div>
          <h1 class="bt-title">{{ t('admin.pelicanTest.title') }}</h1>
          <p class="bt-muted bt-small">{{ t('admin.batchTest.design.workbenchHint') }}</p>
        </div>
      </div>
      <span class="bt-badge bt-queue-note"><Icon name="sync" size="xs" />{{ t('admin.pelicanTest.queueHint') }}</span>
    </header>

    <nav class="bt-segments bt-mobile-nav" :aria-label="t('admin.pelicanTest.title')">
      <button :aria-pressed="mobilePanel === 'config'" @click="mobilePanel = 'config'"><Icon name="edit" size="sm" />{{ t('admin.pelicanTest.configuration') }}</button>
      <button :aria-pressed="mobilePanel === 'results'" @click="mobilePanel = 'results'"><Icon name="grid" size="sm" />{{ t('admin.pelicanTest.results') }}<span v-if="runs.length" class="bt-count">{{ done }}/{{ runs.length }}</span></button>
    </nav>
    <div v-if="error" class="bt-notice" role="alert"><Icon name="exclamationCircle" size="sm" /><span>{{ error }}</span></div>

    <div class="bt-workspace" :data-panel="mobilePanel">
      <section class="bt-panel bt-config-panel" :aria-label="t('admin.pelicanTest.configuration')">
        <div class="bt-group-config">
          <label class="bt-field">
            <span>{{ t('admin.pelicanTest.group') }}</span>
            <select v-model.number="groupId" class="bt-input" :disabled="loading" @change="changeGroup">
              <option :value="0">{{ t('admin.pelicanTest.chooseGroup') }}</option>
              <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option>
            </select>
          </label>
          <details class="bt-key-details">
            <summary class="bt-row bt-between bt-muted bt-small">
              <span class="bt-key-name">{{ groupKeys.find(k => k.id === keyId)?.name || t('admin.pelicanTest.chooseKey') }}</span>
              <span class="bt-row">{{ t('admin.batchTest.design.changeKey') }}<Icon name="chevronDown" size="xs" /></span>
            </summary>
            <select v-model.number="keyId" class="bt-input" :aria-label="t('admin.pelicanTest.key')" :disabled="!groupId || loading" @change="loadModels()">
              <option :value="0">{{ t('admin.pelicanTest.chooseKey') }}</option>
              <option v-for="key in groupKeys" :key="key.id" :value="key.id">{{ key.name }}</option>
            </select>
          </details>
        </div>

        <div class="bt-config-tabs bt-segments" :aria-label="t('admin.pelicanTest.configuration')">
          <button :aria-pressed="configTab === 'models'" @click="configTab = 'models'"><Icon name="grid" size="sm" />{{ t('admin.pelicanTest.chooseModel') }}<span class="bt-count">{{ selected.length }}</span></button>
          <button :aria-pressed="configTab === 'prompt'" @click="configTab = 'prompt'"><Icon name="edit" size="sm" />{{ t('admin.pelicanTest.promptLabel') }}<span class="bt-format">{{ outputMode === 'svg' ? 'SVG' : t('admin.batchTest.design.textFormat') }}</span></button>
        </div>

        <div v-show="configTab === 'models'" class="bt-config-body bt-scroll">
          <div v-if="!keys.length && !loading" class="bt-notice"><Icon name="infoCircle" size="sm" /><span>{{ t('admin.pelicanTest.noKeys') }}</span></div>
          <BatchModelPicker v-model:selected="selected" :models="modelList.map(id => ({ id }))" :loading="loadingModels" :limit="50" />
          <button class="bt-prompt-summary" @click="configTab = 'prompt'">
            <span class="bt-row bt-between"><span class="bt-heading">{{ t('admin.batchTest.design.currentPrompt') }}</span><Icon name="arrowRight" size="sm" /></span>
            <span class="bt-prompt-snippet">{{ prompt || t('admin.pelicanTest.promptLabel') }}</span>
          </button>
        </div>

        <div v-show="configTab === 'prompt'" class="bt-config-body bt-scroll bt-prompt-body">
          <label class="bt-field">
            <span>{{ t('admin.pelicanTest.template') }}</span>
            <select v-model.number="templateId" class="bt-input" :disabled="loading || saving" @change="chooseTemplate">
              <option :value="0">{{ t('admin.pelicanTest.builtin') }}</option>
              <option v-for="item in templates" :key="item.id" :value="item.id">{{ item.name }}{{ item.is_default ? ` · ${t('admin.pelicanTest.default')}` : '' }}</option>
            </select>
          </label>
          <div class="bt-segments" role="group" :aria-label="t('admin.pelicanTest.outputMode')">
            <button v-for="mode in (['svg', 'text'] as const)" :key="mode" :aria-pressed="outputMode === mode" @click="outputMode = mode">
              <Icon :name="mode === 'svg' ? 'grid' : 'terminal'" size="sm" />{{ t(`admin.pelicanTest.mode.${mode}`) }}
            </button>
          </div>
          <label class="bt-field bt-prompt-editor">
            <span class="bt-row bt-between">{{ t('admin.pelicanTest.promptLabel') }}<span class="bt-muted bt-mono bt-small">{{ prompt.length }}/16000</span></span>
            <textarea v-model="prompt" class="bt-input bt-mono" maxlength="16000" :disabled="loading" spellcheck="false" />
          </label>
          <p class="bt-muted bt-small">{{ t('admin.pelicanTest.promptHint') }}</p>
          <details class="bt-template-editor">
            <summary class="bt-link">{{ t('admin.pelicanTest.manageTemplate') }}</summary>
            <div class="bt-stack">
              <input v-model="templateName" class="bt-input" maxlength="100" :placeholder="t('admin.pelicanTest.templateName')" :aria-label="t('admin.pelicanTest.templateName')" />
              <label class="bt-row bt-small"><input v-model="isDefault" type="checkbox" />{{ t('admin.pelicanTest.setDefault') }}</label>
              <div class="bt-row bt-wrap">
                <button class="bt-button" :disabled="saving || !templateName.trim() || !prompt.trim()" @click="saveTemplate()">{{ templateId ? t('admin.pelicanTest.updateTemplate') : t('admin.pelicanTest.saveTemplate') }}</button>
                <button v-if="templateId" class="bt-button" :disabled="saving || !templateName.trim() || !prompt.trim()" @click="saveTemplate(true)">{{ t('admin.pelicanTest.saveAs') }}</button>
                <button v-if="templateId" class="bt-link bt-danger" :disabled="saving" @click="deleting = !deleting">{{ t('admin.pelicanTest.deleteTemplate') }}</button>
              </div>
              <div v-if="deleting" class="bt-notice"><span>{{ t('admin.pelicanTest.deleteConfirm') }} <button class="bt-link bt-danger" :disabled="saving" @click="deleteTemplate">{{ t('admin.pelicanTest.confirmDelete') }}</button></span></div>
            </div>
          </details>
        </div>

        <footer class="bt-config-footer">
          <button class="bt-button bt-primary" :disabled="loading || loadingModels || submitting || !!pending.length || !selected.length || !prompt.trim() || !keyId" @click="submit()">
            <Icon :name="submitting ? 'sync' : 'play'" size="sm" />
            {{ submitting ? t('common.loading') : t('admin.batchTest.start', { count: selected.length }) }}
          </button>
          <p class="bt-muted">{{ t('admin.pelicanTest.billingHint') }}</p>
        </footer>
      </section>

      <section class="bt-panel bt-results-panel">
        <header class="bt-panel-head bt-results-head">
          <div class="bt-tabs">
            <button :aria-pressed="tab === 'results'" @click="tab = 'results'">{{ t('admin.pelicanTest.results') }}<span v-if="runs.length" class="bt-count">{{ runs.length }}</span></button>
            <button :aria-pressed="tab === 'history'" @click="tab = 'history'">{{ t('admin.pelicanTest.history') }}</button>
          </div>
          <span v-if="pending.length" class="bt-status" data-status="running">{{ t('admin.pelicanTest.status.running') }}</span>
        </header>

        <template v-if="tab === 'results'">
          <div v-if="runs.length" class="bt-results-tools">
            <div class="bt-row bt-between bt-wrap">
              <span class="bt-muted bt-small">{{ t('admin.batchTest.progress', { done, total: runs.length, success: successes, failed: failedRuns.length }) }}</span>
              <div class="bt-row">
                <button class="bt-button" @click="copyResults"><Icon name="copy" size="sm" />{{ t('admin.batchTest.copy') }}</button>
                <button v-if="pending.length && batchId" class="bt-button" @click="cancel()">{{ t('admin.batchTest.stop') }}</button>
                <button v-else-if="failedRuns.length" class="bt-button" :disabled="submitting" @click="retry(failedRuns)">{{ t('admin.batchTest.retryFailed') }}</button>
              </div>
            </div>
            <div class="bt-progress" role="progressbar" :aria-valuenow="done" :aria-valuemin="0" :aria-valuemax="runs.length" :aria-label="t('admin.pelicanTest.results')"><span :style="{ width: `${done / runs.length * 100}%` }" /></div>
            <div class="bt-row bt-between bt-wrap">
              <button class="bt-link" :disabled="compareIds.length < 2" :title="t('admin.pelicanTest.compareHint')" @click="comparing = !comparing; onlyFailed = false">{{ comparing ? t('admin.pelicanTest.showAll') : t('admin.pelicanTest.compareSelected', { count: compareIds.length }) }}</button>
              <label class="bt-row bt-muted bt-small"><input v-model="onlyFailed" type="checkbox" @change="comparing = false" />{{ t('admin.batchTest.onlyFailed') }}</label>
            </div>
          </div>
          <div v-if="runs.length" class="bt-scroll bt-results-content">
            <div class="bt-result-grid" :class="{ 'is-single': visibleRuns.length === 1 }">
              <PelicanResultCard v-for="run in visibleRuns" :key="run.id" :run="run" :selected="compareIds.includes(run.id)" :busy="submitting || !!pending.length" @compare="toggleCompare(run.id)" @restore="restoreSettings(run)" @cancel="cancel(run)" @retry="retry([run])" />
            </div>
            <p v-if="!visibleRuns.length" class="bt-empty bt-muted">{{ t('admin.pelicanTest.noFailures') }}</p>
          </div>
          <div v-else class="bt-empty bt-results-empty">
            <div class="bt-empty-art" aria-hidden="true"><span><Icon name="terminal" size="md" /></span><span><Icon name="grid" size="lg" /></span><span><Icon name="chat" size="md" /></span></div>
            <h2>{{ t('admin.batchTest.design.compareTitle') }}</h2>
            <p>{{ t('admin.batchTest.design.compareHint') }}</p>
            <div v-if="selected.length" class="bt-selected-preview"><span v-for="model in selected.slice(0, 4)" :key="model" class="bt-badge bt-mono">{{ model }}</span><span v-if="selected.length > 4" class="bt-badge">+{{ selected.length - 4 }}</span></div>
            <button v-else class="bt-button" @click="mobilePanel = 'config'; configTab = 'models'"><Icon name="plus" size="sm" />{{ t('admin.pelicanTest.chooseModel') }}</button>
          </div>
        </template>

        <template v-else>
          <div class="bt-history-filter bt-row">
            <div class="bt-search bt-spacer"><Icon name="search" size="sm" /><input v-model="historyFilter" class="bt-input" :placeholder="t('admin.pelicanTest.filterModel')" :aria-label="t('admin.pelicanTest.filterModel')" @keyup.enter="loadHistory()" /></div>
            <button class="bt-button" :disabled="loadingHistory" @click="loadHistory()">{{ t('admin.pelicanTest.filter') }}</button>
          </div>
          <div class="bt-scroll bt-history-content">
            <div v-if="!history.length" class="bt-empty bt-muted">{{ loadingHistory ? t('common.loading') : t('admin.pelicanTest.empty') }}</div>
            <article v-for="run in history" :key="run.id" class="bt-history-row">
              <span class="bt-history-icon"><Icon :name="run.output_mode === 'text' ? 'terminal' : 'grid'" size="sm" /></span>
              <button class="bt-spacer bt-history-open" @click="showBatch(run.batch_id || '', run.id)">
                <strong>{{ run.model }}</strong>
                <span>{{ run.group_name }} · {{ new Date(run.created_at).toLocaleString() }}</span>
              </button>
              <span class="bt-status" :data-status="run.status">{{ t(`admin.pelicanTest.status.${run.status}`) }}</span>
              <button class="bt-button" @click="restoreSettings(run)">{{ t('admin.pelicanTest.restore') }}</button>
            </article>
            <button v-if="nextBeforeId" class="bt-button bt-load-more" :disabled="loadingHistory" @click="loadHistory(true)">{{ t('admin.pelicanTest.loadMore') }}</button>
          </div>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import BatchModelPicker from '../BatchModelPicker.vue'
import PelicanResultCard from './PelicanResultCard.vue'
import { usePelicanWorkbench } from './usePelicanWorkbench'
import type { PelicanRun } from '@/api/admin/pelicanTests'
import '../batch-test.css'

const configTab = ref<'models' | 'prompt'>('models')
const mobilePanel = ref<'config' | 'results'>('config')
const { t, keys, templates, groupId, keyId, selected, prompt, outputMode, templateId, templateName, isDefault, deleting,
  loading, loadingModels, submitting, saving, error, batchId, runs, history, nextBeforeId, loadingHistory, tab, compareIds, comparing,
  onlyFailed, historyFilter, groupKeys, groups, modelList, pending, failedRuns, done, successes, visibleRuns,
  chooseTemplate, saveTemplate, deleteTemplate, loadModels, changeGroup, loadHistory, showBatch, submit, retry,
  cancel, restore, toggleCompare, copyResults } = usePelicanWorkbench()

watch(() => runs.value[0]?.id, id => { if (id) mobilePanel.value = 'results' })
async function restoreSettings(run: PelicanRun) {
  await restore(run)
  configTab.value = 'prompt'
  mobilePanel.value = 'config'
}
</script>

<style scoped>
.bt-workbench { display: flex; flex-direction: column; gap: 16px; height: calc(100dvh - 112px); min-height: 0; }
.bt-page-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; flex-shrink: 0; }
.bt-page-head p { margin-top: 3px; }
.bt-page-icon { display: grid; place-items: center; width: 42px; height: 42px; border-radius: 11px; color: var(--bt-accent); background: var(--bt-tint); border: 1px solid var(--bt-selected); }
.bt-workspace { display: grid; grid-template-columns: 370px minmax(0, 1fr); gap: 20px; min-height: 0; flex: 1; }
.bt-config-panel, .bt-results-panel { display: flex; flex-direction: column; }
.bt-group-config { padding: 18px 18px 12px; border-bottom: 1px solid var(--bt-line); }
.bt-key-details { margin-top: 9px; }
.bt-key-details summary { list-style: none; cursor: pointer; gap: 12px; }
.bt-key-details summary::-webkit-details-marker { display: none; }
.bt-key-details select { margin-top: 10px; }
.bt-key-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bt-key-details summary > span:last-child { flex-shrink: 0; }
.bt-config-tabs { margin: 14px 18px 0; flex-shrink: 0; }
.bt-format { font-size: 10px; font-weight: 400; opacity: .75; }
.bt-config-body { display: flex; flex-direction: column; gap: 16px; padding: 16px 18px; }
.bt-config-body :deep(.bt-model-picker) { flex: 1; min-height: 190px; }
.bt-config-body :deep(.bt-model-list) { grid-template-columns: 1fr; flex: 1; }
.bt-prompt-summary { display: block; flex-shrink: 0; padding: 13px 14px; text-align: left; border: 1px solid var(--bt-line); border-radius: 9px; background: var(--bt-soft); color: var(--bt-muted); }
.bt-prompt-summary:hover { border-color: var(--bt-selected); }
.bt-prompt-snippet { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; margin-top: 6px; font-size: 12px; line-height: 1.7; }
.bt-prompt-editor { flex: 1; min-height: 210px; }
.bt-prompt-editor textarea { flex: 1; resize: vertical; min-height: 180px; font-size: 12px; line-height: 1.9; padding: 13px; }
.bt-template-editor summary { cursor: pointer; }
.bt-template-editor > div { padding-top: 14px; }
.bt-config-footer { padding: 15px 18px; border-top: 1px solid var(--bt-line); flex-shrink: 0; }
.bt-config-footer > button { width: 100%; }
.bt-config-footer > p { margin-top: 9px; text-align: center; font-size: 10px; }
.bt-results-head { padding-block: 0; }
.bt-results-tools { display: flex; flex-direction: column; gap: 14px; padding: 16px 18px; border-bottom: 1px solid var(--bt-line); }
.bt-results-content { padding: 18px; background: var(--bt-soft); }
.bt-result-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 290px), 1fr)); align-items: start; gap: 16px; }
.bt-result-grid.is-single { grid-template-columns: minmax(0, 1fr); }
.bt-results-empty { background: var(--bt-soft); }
.bt-selected-preview { display: flex; flex-wrap: wrap; justify-content: center; gap: 7px; max-width: 400px; }
.bt-history-filter { padding: 16px 18px; border-bottom: 1px solid var(--bt-line); }
.bt-history-content { padding: 6px 18px 18px; }
.bt-history-row { display: flex; align-items: center; gap: 12px; padding: 17px 0; border-bottom: 1px solid var(--bt-line); }
.bt-history-icon { display: grid; place-items: center; width: 36px; height: 36px; flex-shrink: 0; border: 1px solid var(--bt-line); border-radius: 8px; color: var(--bt-muted); background: var(--bt-soft); }
.bt-history-open { text-align: left; }
.bt-history-open strong { display: block; font-size: 13px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bt-history-open span { display: block; margin-top: 4px; color: var(--bt-muted); font-size: 11px; }
.bt-load-more { width: 100%; margin-top: 16px; }
.bt-mobile-nav { display: none; }
@media (min-width: 768px) { .bt-workbench { height: calc(100dvh - 128px); } }
@media (min-width: 1024px) { .bt-workbench { height: calc(100dvh - 144px); } }
@media (min-width: 1200px) and (max-height: 820px) {
  .bt-page-head { display: none; }
  .bt-prompt-summary { display: none; }
  .bt-config-body :deep(.bt-model-list) { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .bt-config-body :deep(.bt-model) { min-height: 54px; padding: 10px; gap: 8px; }
  .bt-config-body :deep(.bt-model-mark) { display: none; }
  .bt-config-body :deep(.bt-model-name) { font-size: 12px; }
}
@media (max-width: 1199px) {
  .bt-workspace { display: flex; }
  .bt-config-panel, .bt-results-panel { flex: 1; }
  .bt-mobile-nav { display: flex; flex-shrink: 0; }
  .bt-workspace[data-panel='config'] .bt-results-panel, .bt-workspace[data-panel='results'] .bt-config-panel { display: none; }
  .bt-config-body :deep(.bt-model-list) { grid-template-columns: repeat(auto-fill, minmax(min(100%, 230px), 1fr)); }
  .bt-config-body :deep(.bt-model-picker) { flex: 1; }
  .bt-prompt-editor { min-height: 220px; }
}
@media (max-width: 639px) {
  .bt-queue-note { display: none; }
  .bt-page-head p { max-width: 260px; font-size: 11px; }
  .bt-history-row { flex-wrap: wrap; gap: 9px; }
  .bt-history-row .bt-status { margin-left: 48px; }
  .bt-history-row > .bt-button { margin-left: auto; }
}
</style>
