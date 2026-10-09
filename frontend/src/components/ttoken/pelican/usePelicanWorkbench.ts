import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import api, { type PelicanKeyOption, type PelicanRun, type PelicanTemplate, type PelicanBatchInput } from '@/api/admin/pelicanTests'

export function usePelicanWorkbench() {
  const { t } = useI18n(), auth = useAuthStore(), app = useAppStore()
  const { copyToClipboard } = useClipboard()
  const keys = ref<PelicanKeyOption[]>([]), templates = ref<PelicanTemplate[]>([])
  const groupId = ref(0), keyId = ref(0), modelList = ref<string[]>([]), selected = ref<string[]>([]), search = ref('')
  const builtinPrompt = ref('Generate an SVG of a pelican riding a bicycle'), prompt = ref(builtinPrompt.value)
  const outputMode = ref<'svg' | 'text'>('svg'), templateId = ref(0), templateName = ref(''), isDefault = ref(false)
  const loading = ref(true), loadingModels = ref(false), submitting = ref(false), saving = ref(false), error = ref('')
  const batchId = ref(''), runs = ref<PelicanRun[]>([]), history = ref<PelicanRun[]>([]), nextBeforeId = ref(0)
  const loadingHistory = ref(false), tab = ref<'results' | 'history'>('results'), compareIds = ref<number[]>([]), comparing = ref(false)
  const onlyFailed = ref(false), historyFilter = ref(''), deleting = ref(false)
  const groupKeys = computed(() => keys.value.filter(k => k.group_id === groupId.value))
  const groups = computed(() => [...new Map(keys.value.map(k => [k.group_id, { id: k.group_id, name: k.group_name }])).values()])
  const filteredModels = computed(() => modelList.value.filter(m => m.toLowerCase().includes(search.value.toLowerCase())))
  const active = (run: PelicanRun) => ['queued', 'running'].includes(run.status)
  const failed = (run: PelicanRun) => ['error', 'invalid_svg', 'interrupted'].includes(run.status)
  const pending = computed(() => runs.value.filter(active))
  const failedRuns = computed(() => runs.value.filter(failed))
  const done = computed(() => runs.value.length - pending.value.length)
  const successes = computed(() => runs.value.filter(r => r.status === 'success').length)
  const visibleRuns = computed(() => runs.value.filter(r => (!comparing.value || compareIds.value.includes(r.id)) && (!onlyFailed.value || failed(r))))
  let alive = true, ready = false, modelGeneration = 0, resultGeneration = 0, historyGeneration = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  const storageKey = `ttoken:pelican-workbench:${auth.user?.id ?? 'admin'}`
  function message(e: unknown) {
    return e && typeof e === 'object' && 'message' in e ? String(e.message) : t('admin.pelicanTest.loadFailed')
  }
  function saveDraft() {
    if (!ready || !alive) return
    try { localStorage.setItem(storageKey, JSON.stringify({ groupId: groupId.value, keyId: keyId.value, selected: selected.value, prompt: prompt.value, outputMode: outputMode.value, templateId: templateId.value, batchId: batchId.value })) } catch { /* Private browsing or quota: keep the live form usable. */ }
  }
  watch([groupId, keyId, selected, prompt, outputMode, templateId, batchId], saveDraft, { deep: true })
  function chooseTemplate() {
    const item = templates.value.find(item => item.id === templateId.value)
    prompt.value = item?.prompt ?? builtinPrompt.value
    outputMode.value = item?.output_mode ?? 'svg'
    templateName.value = item?.name ?? ''
    isDefault.value = item?.is_default ?? false
    deleting.value = false
  }
  async function saveTemplate(asNew = false) {
    if (saving.value || !templateName.value.trim() || !prompt.value.trim()) return
    saving.value = true
    try {
      const saved = await api.saveTemplate({ name: templateName.value.trim(), prompt: prompt.value, output_mode: outputMode.value, is_default: isDefault.value }, asNew ? undefined : templateId.value || undefined)
      templates.value = await api.templates()
      templateId.value = saved.id
      app.showSuccess(t('admin.pelicanTest.templateSaved'))
    } catch (e) { app.showError(message(e)) }
    finally { saving.value = false }
  }
  async function deleteTemplate() {
    if (!templateId.value || saving.value) return
    saving.value = true
    try {
      await api.deleteTemplate(templateId.value)
      templates.value = await api.templates()
      templateId.value = 0; chooseTemplate()
    } catch (e) { app.showError(message(e)) }
    finally { saving.value = false }
  }
  async function loadModels(keep: string[] = []) {
    const token = ++modelGeneration, id = keyId.value
    modelList.value = []; selected.value = []; loadingModels.value = !!id; error.value = ''
    if (!id) return
    try {
      const models = await api.models(id)
      if (!alive || token !== modelGeneration) return
      modelList.value = [...new Set(models)]
      selected.value = keep.filter(m => models.includes(m))
    } catch (e) { if (alive && token === modelGeneration) error.value = message(e) }
    finally { if (alive && token === modelGeneration) loadingModels.value = false }
  }
  async function changeGroup() {
    keyId.value = groupKeys.value.find(k => k.id === keyId.value)?.id ?? groupKeys.value[0]?.id ?? 0
    search.value = ''
    await loadModels()
  }
  function selectVisible() { selected.value = [...new Set([...selected.value, ...filteredModels.value])].slice(0, 50) }
  async function loadHistory(more = false) {
    const token = ++historyGeneration
    loadingHistory.value = true
    try {
      const result = await api.listRuns(undefined, historyFilter.value.trim(), more ? nextBeforeId.value : undefined)
      if (!alive || token !== historyGeneration) return
      history.value = more ? [...history.value, ...result.items] : result.items
      nextBeforeId.value = result.next_before_id
    } catch (e) { if (alive && token === historyGeneration) app.showError(message(e)) }
    finally { if (alive && token === historyGeneration) loadingHistory.value = false }
  }
  async function showBatch(id: string, singleId?: number) {
    const token = ++resultGeneration
    clearTimeout(timer)
    batchId.value = id
    runs.value = []; compareIds.value = []; comparing.value = false; onlyFailed.value = false
    tab.value = 'results'; error.value = ''
    let settled = 0
    async function refresh() {
      try {
        const items = id ? await api.getBatch(id) : singleId ? [await api.getRun(singleId)] : []
        if (!alive || token !== resultGeneration) return
        runs.value = items
        error.value = ''
        if (items.some(active)) settled = 0
        else settled++
        // One extra read lets asynchronous usage-log writes populate billing data.
        if (items.some(active) || settled < 2) timer = setTimeout(refresh, 2500)
        else void loadHistory()
      } catch (e) {
        if (!alive || token !== resultGeneration) return
        error.value = message(e)
        timer = setTimeout(refresh, 5000)
      }
    }
    await refresh()
  }
  async function submit(input?: PelicanBatchInput) {
    if (submitting.value) return
    const data = input ?? { key_id: keyId.value, models: selected.value, prompt: prompt.value, output_mode: outputMode.value }
    if (!data.key_id || !data.models.length || !data.prompt.trim()) return
    submitting.value = true
    try {
      const result = await api.createBatch(data)
      if (!alive) return
      await showBatch(result.batch_id)
      void loadHistory()
    } catch (e) { if (alive) app.showError(message(e)) }
    finally { if (alive) submitting.value = false }
  }
  async function retry(items: PelicanRun[]) {
    const first = items[0]
    if (!first) return
    await submit({ key_id: first.api_key_id, models: items.map(r => r.model), prompt: first.prompt, output_mode: first.output_mode || 'svg' })
  }
  async function cancel(run?: PelicanRun) {
    try {
      if (run) await api.cancelRun(run.id)
      else if (batchId.value) await api.cancelBatch(batchId.value)
      // The polling loop will update the current batch without resetting selection.
      if (alive) {
        const id = batchId.value, token = resultGeneration
        const items = id ? await api.getBatch(id) : run ? [await api.getRun(run.id)] : []
        if (alive && token === resultGeneration) runs.value = items
      }
    } catch (e) { app.showError(message(e)) }
  }
  async function restore(run: PelicanRun) {
    const key = keys.value.find(k => k.id === run.api_key_id && k.group_id === run.group_id)
    if (!key) { app.showError(t('admin.pelicanTest.keyUnavailable')); return }
    groupId.value = key.group_id; keyId.value = key.id; prompt.value = run.prompt
    outputMode.value = run.output_mode || 'svg'; templateId.value = 0; templateName.value = ''; isDefault.value = false
    await loadModels([run.model])
    app.showSuccess(t('admin.pelicanTest.restored'))
  }
  function toggleCompare(id: number) {
    if (compareIds.value.includes(id)) compareIds.value = compareIds.value.filter(v => v !== id)
    else if (compareIds.value.length < 4) compareIds.value = [...compareIds.value, id]
    else app.showError(t('admin.pelicanTest.limit'))
    if (compareIds.value.length < 2) comparing.value = false
  }
  function copyResults() {
    void copyToClipboard(runs.value.map(r => `${r.model}\t${t(`admin.pelicanTest.status.${r.status}`)}\t${r.duration_ms}ms\n${r.response_text || r.error_message || ''}`).join('\n\n'), t('admin.pelicanTest.copied'))
  }
  onMounted(async () => {
    try {
      const [options, savedTemplates] = await Promise.all([api.options(), api.templates()])
      if (!alive) return
      keys.value = options.keys; builtinPrompt.value = options.prompt; templates.value = savedTemplates
      let draft: Record<string, unknown> = {}
      try { draft = JSON.parse(localStorage.getItem(storageKey) || '{}') || {} } catch { /* Ignore obsolete draft data. */ }
      templateId.value = templates.value.find(item => item.id === Number(draft.templateId))?.id ?? templates.value.find(item => item.is_default)?.id ?? 0
      chooseTemplate()
      if (typeof draft.prompt === 'string') prompt.value = draft.prompt.slice(0, 16000)
      if (draft.outputMode === 'svg' || draft.outputMode === 'text') outputMode.value = draft.outputMode
      groupId.value = groups.value.find(g => g.id === Number(draft.groupId))?.id ?? groups.value[0]?.id ?? 0
      keyId.value = groupKeys.value.find(k => k.id === Number(draft.keyId))?.id ?? groupKeys.value[0]?.id ?? 0
      await loadModels(Array.isArray(draft.selected) ? draft.selected.filter((m): m is string => typeof m === 'string') : [])
      if (!alive) return
      ready = true
      if (typeof draft.batchId === 'string' && /^[0-9a-f-]{36}$/i.test(draft.batchId)) void showBatch(draft.batchId)
      void loadHistory()
    } catch (e) { if (alive) error.value = message(e) }
    finally { if (alive) loading.value = false }
  })
  onBeforeUnmount(() => { alive = false; modelGeneration++; resultGeneration++; historyGeneration++; clearTimeout(timer) })
  return { t, keys, templates, groupId, keyId, selected, search, prompt, outputMode, templateId, templateName, isDefault, deleting,
    loading, loadingModels, submitting, saving, error, batchId, runs, history, nextBeforeId, loadingHistory, tab, compareIds, comparing,
    onlyFailed, historyFilter, groupKeys, groups, filteredModels, modelList, pending, failedRuns, done, successes, visibleRuns,
    active, chooseTemplate, saveTemplate, deleteTemplate, loadModels, changeGroup, selectVisible, loadHistory, showBatch, submit, retry,
    cancel, restore, toggleCompare, copyResults }
}
