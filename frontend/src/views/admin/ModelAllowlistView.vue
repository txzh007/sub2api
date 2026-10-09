<template>
  <AppLayout>
    <div class="allowlist-page flex min-h-0 flex-col gap-4">
      <header class="flex shrink-0 flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('admin.modelAllowlist.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.modelAllowlist.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary text-sm" :disabled="checkingPlaza || saving" @click="openPlaza">{{ checkingPlaza ? t('common.loading') : t('admin.modelAllowlist.openPlaza') }}</button>
      </header>

      <div v-if="loading" class="flex min-h-0 flex-1 items-center justify-center text-gray-500" role="status">{{ t('common.loading') }}</div>
      <div v-else-if="loadError" class="flex flex-1 flex-col items-center justify-center gap-4">
        <p class="text-sm text-red-600" role="alert">{{ loadError }}</p>
        <button class="btn btn-secondary" @click="loadGroups">{{ t('admin.modelAllowlist.retry') }}</button>
      </div>
      <div v-else-if="!groups.length" class="flex flex-1 items-center justify-center text-gray-500">{{ t('admin.modelAllowlist.noGroups') }}</div>
      <template v-else>
        <label class="shrink-0 md:hidden">
          <span class="sr-only">{{ t('admin.modelAllowlist.groups') }}</span>
          <select :value="selectedId" class="input w-full" :disabled="saving" @change="selectGroup(Number(($event.target as HTMLSelectElement).value))">
            <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}{{ isDirty(group.id) ? ' •' : '' }}</option>
          </select>
        </label>

        <div class="flex min-h-0 flex-1 gap-4">
          <aside class="hidden w-64 shrink-0 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 md:flex">
            <div class="shrink-0 border-b border-gray-100 p-4 dark:border-dark-700">
              <div class="mb-3 flex items-center justify-between text-sm font-semibold text-gray-800 dark:text-gray-100">
                <span>{{ t('admin.modelAllowlist.groups') }}</span><span class="text-xs tabular-nums text-gray-400">{{ groups.length }}</span>
              </div>
              <input v-model="groupSearch" class="input w-full text-sm" :placeholder="t('admin.modelAllowlist.searchGroups')" :aria-label="t('admin.modelAllowlist.searchGroups')" />
            </div>
            <nav class="min-h-0 flex-1 space-y-1 overflow-y-auto p-2" :aria-label="t('admin.modelAllowlist.groups')">
              <button v-for="group in filteredGroups" :key="group.id" type="button" class="w-full rounded-lg border px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :class="selectedId === group.id ? 'border-primary-200 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-transparent hover:bg-gray-50 dark:hover:bg-dark-700'" :aria-current="selectedId === group.id ? 'true' : undefined" :disabled="saving" @click="selectGroup(group.id)">
                <div class="flex items-center gap-2">
                  <span class="min-w-0 flex-1 truncate text-sm font-semibold text-gray-900 dark:text-white" :title="group.name">{{ group.name }}</span>
                  <span v-if="isDirty(group.id)" class="h-2 w-2 shrink-0 rounded-full bg-amber-500" :title="t('admin.modelAllowlist.unsaved')"><span class="sr-only">{{ t('admin.modelAllowlist.unsaved') }}</span></span>
                </div>
                <div class="mt-1.5 flex items-center justify-between gap-2 text-xs">
                  <span class="truncate text-gray-400">{{ group.platform }}<span v-if="group.status !== 'active'"> · {{ t('admin.modelAllowlist.inactive') }}</span></span>
                  <span :class="drafts[group.id]?.enabled ? 'text-primary-600 dark:text-primary-400' : 'text-gray-400'">{{ drafts[group.id]?.enabled ? t('admin.modelAllowlist.itemCount', { count: drafts[group.id]?.models.length }) : t('admin.modelAllowlist.unrestricted') }}</span>
                </div>
              </button>
              <p v-if="!filteredGroups.length" class="p-4 text-center text-sm text-gray-400">{{ t('admin.modelAllowlist.noResults') }}</p>
            </nav>
          </aside>

          <section v-if="currentGroup && currentDraft" class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" :aria-label="currentGroup.name">
            <div class="shrink-0 border-b border-gray-100 p-4 dark:border-dark-700">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="min-w-0">
                  <h2 class="truncate font-semibold text-gray-900 dark:text-white">{{ currentGroup.name }}</h2>
                  <p class="mt-1 text-xs text-gray-400">{{ currentGroup.platform }} · #{{ currentGroup.id }}</p>
                </div>
                <label class="flex cursor-pointer items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
                  <input v-model="currentDraft.enabled" type="checkbox" class="checkbox" :disabled="saving" />
                  {{ t('admin.modelAllowlist.enable') }}
                </label>
              </div>
              <p class="mt-3 rounded-lg px-3 py-2 text-xs leading-relaxed" :class="currentDraft.enabled ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'bg-gray-50 text-gray-500 dark:bg-dark-700 dark:text-gray-400'">
                {{ currentDraft.enabled ? t('admin.modelAllowlist.enabledHint') : t('admin.modelAllowlist.disabledHint') }}
              </p>
              <p v-if="currentGroup.status !== 'active'" class="mt-2 text-xs text-amber-600 dark:text-amber-400">{{ t('admin.modelAllowlist.inactiveHint') }}</p>
            </div>

            <div class="shrink-0 space-y-3 border-b border-gray-100 p-4 dark:border-dark-700">
              <div class="flex gap-2">
                <input v-model="modelSearch" class="input min-w-0 flex-1 text-sm" :placeholder="t('admin.modelAllowlist.searchModels')" :aria-label="t('admin.modelAllowlist.searchModels')" />
                <button class="btn btn-secondary shrink-0 text-sm" :disabled="saving" @click="customOpen = true">{{ t('admin.modelAllowlist.addCustom') }}</button>
              </div>
              <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs">
                <button class="font-medium text-primary-600 hover:underline disabled:opacity-40 dark:text-primary-400" :disabled="saving || !visibleModels.length" @click="setVisibleSelection(true)">{{ t('admin.modelAllowlist.selectVisible') }}</button>
                <button class="text-gray-500 hover:underline disabled:opacity-40 dark:text-gray-400" :disabled="saving || !visibleModels.length" @click="setVisibleSelection(false)">{{ t('admin.modelAllowlist.clearVisible') }}</button>
                <label class="flex cursor-pointer items-center gap-1.5 text-gray-500 dark:text-gray-400"><input v-model="selectedOnly" type="checkbox" class="checkbox" />{{ t('admin.modelAllowlist.selectedOnly') }}</label>
                <span class="ml-auto tabular-nums text-gray-400">{{ t('admin.modelAllowlist.selectionCount', { selected: currentDraft.models.length, total: currentModels.length }) }}</span>
              </div>
            </div>

            <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain" :aria-busy="candidateLoading[selectedId]">
              <div v-if="candidateErrors[selectedId]" class="m-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" role="alert">
                {{ candidateErrors[selectedId] }}
                <button class="ml-2 underline" :disabled="candidateLoading[selectedId]" @click="loadCandidates(currentGroup, true)">{{ t('admin.modelAllowlist.retry') }}</button>
              </div>
              <p v-if="candidateLoading[selectedId]" class="p-4 text-center text-sm text-gray-400" role="status">{{ t('common.loading') }}</p>
              <div class="grid grid-cols-1 gap-px bg-gray-100 dark:bg-dark-700 xl:grid-cols-2">
                <label v-for="model in visibleModels" :key="model" class="flex min-w-0 cursor-pointer items-center gap-3 px-4 py-3 transition-colors" :class="selectedModels.has(model) ? 'bg-primary-50/60 dark:bg-primary-900/10' : 'bg-white hover:bg-gray-50 dark:bg-dark-800 dark:hover:bg-dark-700'">
                  <input type="checkbox" class="checkbox shrink-0" :checked="selectedModels.has(model)" :disabled="saving" @change="toggleModel(model)" />
                  <code class="min-w-0 flex-1 break-all text-xs leading-5 text-gray-700 dark:text-gray-200">{{ model }}</code>
                  <span v-if="model.includes('*')" class="shrink-0 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">{{ t('admin.modelAllowlist.pattern') }}</span>
                </label>
              </div>
              <p v-if="!visibleModels.length && !candidateLoading[selectedId]" class="p-8 text-center text-sm text-gray-400">{{ t('admin.modelAllowlist.noResults') }}</p>
            </div>

            <footer class="shrink-0 space-y-3 border-t border-gray-100 p-4 dark:border-dark-700">
              <p v-if="currentDraft.enabled && !currentDraft.models.length" class="text-xs text-red-600" role="alert">{{ t('admin.modelAllowlist.emptyEnabled') }}</p>
              <p class="text-xs leading-relaxed text-gray-400">{{ t('admin.modelAllowlist.capabilityHint') }}</p>
              <div class="flex flex-wrap items-center gap-2">
                <button class="btn btn-secondary text-sm" :disabled="saving || groups.length < 2" @click="openCopy">{{ t('admin.modelAllowlist.copy') }}</button>
                <button class="btn btn-secondary text-sm" :disabled="saving || !isDirty(selectedId)" @click="resetCurrent">{{ t('admin.modelAllowlist.reset') }}</button>
                <div class="ml-auto flex items-center gap-2">
                  <span v-if="dirtyIds.length" class="hidden text-xs text-amber-600 lg:inline">{{ t('admin.modelAllowlist.pendingCount', { count: dirtyIds.length }) }}</span>
                  <button v-if="dirtyIds.length > 1" class="btn btn-secondary text-sm" :disabled="saving" @click="saveGroups(dirtyIds)">{{ t('admin.modelAllowlist.saveAll') }}</button>
                  <button class="btn btn-primary text-sm" :disabled="saving || !isDirty(selectedId) || (currentDraft.enabled && !currentDraft.models.length)" @click="saveGroups([selectedId])">{{ saving ? t('common.saving') : t('admin.modelAllowlist.save') }}</button>
                </div>
              </div>
            </footer>
          </section>
        </div>
      </template>
    </div>

    <BaseDialog :show="customOpen" :title="t('admin.modelAllowlist.addCustom')" @close="customOpen = false">
      <p class="mb-3 text-sm text-gray-500">{{ t('admin.modelAllowlist.customHint') }}</p>
      <textarea v-model="customText" class="input w-full font-mono text-sm" rows="6" :placeholder="t('admin.modelAllowlist.customPlaceholder')" :aria-label="t('admin.modelAllowlist.addCustom')" />
      <template #footer><div class="flex justify-end gap-2"><button class="btn btn-secondary" @click="customOpen = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="!customText.trim()" @click="addCustom">{{ t('admin.modelAllowlist.addSelected') }}</button></div></template>
    </BaseDialog>

    <BaseDialog :show="copyOpen" :title="t('admin.modelAllowlist.copy')" @close="copyOpen = false">
      <p class="mb-4 text-sm leading-relaxed text-gray-500">{{ t('admin.modelAllowlist.copyHint', { name: currentGroup?.name }) }}</p>
      <input v-model="copySearch" class="input mb-3 w-full" :placeholder="t('admin.modelAllowlist.searchGroups')" :aria-label="t('admin.modelAllowlist.searchGroups')" />
      <div class="max-h-72 space-y-1 overflow-y-auto">
        <label v-for="group in copyGroups" :key="group.id" class="flex cursor-pointer items-center gap-3 rounded-lg p-3 hover:bg-gray-50 dark:hover:bg-dark-700">
          <input v-model="copyTargets" type="checkbox" class="checkbox" :value="group.id" />
          <span class="min-w-0 flex-1 truncate text-sm text-gray-700 dark:text-gray-200">{{ group.name }}<span v-if="isDirty(group.id)" class="ml-2 text-xs text-amber-600">{{ t('admin.modelAllowlist.unsaved') }}</span></span>
          <span class="text-xs text-gray-400">{{ group.platform }}</span>
        </label>
      </div>
      <template #footer><div class="flex justify-end gap-2"><button class="btn btn-secondary" @click="copyOpen = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="!copyTargets.length" @click="copyDraft">{{ t('admin.modelAllowlist.copyDraft', { count: copyTargets.length }) }}</button></div></template>
    </BaseDialog>

    <ConfirmDialog :show="leaveOpen" :title="t('admin.modelAllowlist.unsaved')" :message="t('admin.modelAllowlist.leaveHint')" :confirm-text="t('admin.modelAllowlist.leave')" @confirm="resolveLeave(true)" @cancel="resolveLeave(false)" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminGroup, ModelAllowlist } from '@/types'

type Config = { enabled: boolean; models: string[] }
const { t } = useI18n()
const appStore = useAppStore()
const router = useRouter()
const checkingPlaza = ref(false)
const groups = ref<AdminGroup[]>([])
const drafts = reactive<Record<number, Config>>({})
const saved = reactive<Record<number, Config>>({})
const candidates = reactive<Record<number, string[]>>({})
const customCandidates = reactive<Record<number, string[]>>({})
const candidateLoading = reactive<Record<number, boolean>>({})
const candidateErrors = reactive<Record<number, string>>({})
const selectedId = ref(0)
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const groupSearch = ref('')
const modelSearch = ref('')
const selectedOnly = ref(false)
const customOpen = ref(false)
const customText = ref('')
const copyOpen = ref(false)
const copySearch = ref('')
const copyTargets = ref<number[]>([])
const leaveOpen = ref(false)
let leaveResolver: ((value: boolean) => void) | undefined

function uniqueModels(models: string[]): string[] {
  const seen = new Set<string>()
  return models.map(model => model.trim()).filter(model => {
    const key = model.toLowerCase()
    if (!key || seen.has(key)) return false
    seen.add(key)
    return true
  })
}
function cloneConfig(config?: ModelAllowlist | Config): Config {
  return { enabled: config?.enabled ?? false, models: uniqueModels(config?.models ?? []) }
}
function isDirty(id: number): boolean {
  return JSON.stringify(drafts[id]) !== JSON.stringify(saved[id])
}
const dirtyIds = computed(() => groups.value.filter(group => isDirty(group.id)).map(group => group.id))
const currentGroup = computed(() => groups.value.find(group => group.id === selectedId.value))
const currentDraft = computed(() => drafts[selectedId.value])
const selectedModels = computed(() => new Set(currentDraft.value?.models ?? []))
const currentModels = computed(() => uniqueModels([
  ...(currentDraft.value?.models ?? []),
  ...(saved[selectedId.value]?.models ?? []),
  ...(customCandidates[selectedId.value] ?? []),
  ...(candidates[selectedId.value] ?? [])
]).sort((a, b) => a.localeCompare(b)))
const visibleModels = computed(() => currentModels.value.filter(model =>
  model.toLowerCase().includes(modelSearch.value.trim().toLowerCase()) && (!selectedOnly.value || selectedModels.value.has(model))
))
const matchesGroup = (group: AdminGroup, query: string) => `${group.name} ${group.platform} ${group.id}`.toLowerCase().includes(query.trim().toLowerCase())
const filteredGroups = computed(() => groups.value.filter(group => matchesGroup(group, groupSearch.value)))
const copyGroups = computed(() => groups.value.filter(group => group.id !== selectedId.value && matchesGroup(group, copySearch.value)))

function errorText(error: unknown, fallback: string): string {
  const err = error as { response?: { data?: { message?: string } }; message?: string }
  return err?.response?.data?.message || err?.message || fallback
}
async function openPlaza() {
  if (checkingPlaza.value || saving.value) return
  checkingPlaza.value = true
  try {
    const settings = await appStore.fetchPublicSettings(true)
    if (!settings) {
      appStore.showError(t('admin.modelAllowlist.plazaSettingsFailed'))
      return
    }
    if (!settings.model_plaza_enabled) {
      appStore.showWarning(t('admin.modelAllowlist.plazaDisabled'), 7000)
      return
    }
    await router.push({ path: '/model-plaza', query: { embedded: '1' } })
  } catch (error) {
    appStore.showError(errorText(error, t('admin.modelAllowlist.plazaSettingsFailed')))
  } finally {
    checkingPlaza.value = false
  }
}
async function loadGroups() {
  loading.value = true
  loadError.value = ''
  try {
    groups.value = await adminAPI.groups.getAllIncludingInactive()
    for (const group of groups.value) {
      saved[group.id] = cloneConfig(group.model_allowlist)
      drafts[group.id] = cloneConfig(group.model_allowlist)
    }
    if (groups.value[0]) selectGroup(groups.value[0].id)
  } catch (error) {
    loadError.value = errorText(error, t('admin.modelAllowlist.loadFailed'))
  } finally {
    loading.value = false
  }
}
function selectGroup(id: number) {
  selectedId.value = id
  modelSearch.value = ''
  selectedOnly.value = false
  const group = groups.value.find(item => item.id === id)
  if (group) void loadCandidates(group)
}
async function loadCandidates(group: AdminGroup, force = false) {
  if (candidateLoading[group.id] || (!force && candidates[group.id])) return
  candidateLoading[group.id] = true
  candidateErrors[group.id] = ''
  try {
    candidates[group.id] = uniqueModels(await adminAPI.groups.getModelAllowlistCandidates(group.id))
  } catch (error) {
    candidateErrors[group.id] = errorText(error, t('admin.modelAllowlist.loadFailed'))
  } finally {
    candidateLoading[group.id] = false
  }
}
function toggleModel(model: string) {
  const config = currentDraft.value
  if (!config) return
  config.models = selectedModels.value.has(model) ? config.models.filter(item => item !== model) : [...config.models, model]
}
function setVisibleSelection(selected: boolean) {
  const config = currentDraft.value
  if (!config) return
  const visible = new Set(visibleModels.value)
  config.models = selected ? uniqueModels([...config.models, ...visible]) : config.models.filter(model => !visible.has(model))
}
function addCustom() {
  const config = currentDraft.value
  if (!config) return
  const added = uniqueModels(customText.value.split(/[\n,，]+/))
  customCandidates[selectedId.value] = uniqueModels([...(customCandidates[selectedId.value] ?? []), ...added])
  config.models = uniqueModels([...config.models, ...added])
  customText.value = ''
  customOpen.value = false
}
function resetCurrent() {
  drafts[selectedId.value] = cloneConfig(saved[selectedId.value])
}
function openCopy() {
  copySearch.value = ''
  copyTargets.value = []
  copyOpen.value = true
}
function copyDraft() {
  if (!currentDraft.value) return
  for (const id of copyTargets.value) drafts[id] = cloneConfig(currentDraft.value)
  copyOpen.value = false
  appStore.showSuccess(t('admin.modelAllowlist.copied', { count: copyTargets.value.length }))
}
async function saveGroups(ids: number[]) {
  if (saving.value) return
  const targets = [...ids].filter(isDirty)
  const invalid = targets.find(id => drafts[id]?.enabled && !drafts[id]?.models.length)
  if (invalid !== undefined) {
    selectGroup(invalid)
    appStore.showError(t('admin.modelAllowlist.emptyEnabled'))
    return
  }
  saving.value = true
  let count = 0
  const failures: string[] = []
  try {
    for (const id of targets) {
      try {
        const updated = await adminAPI.groups.update(id, { model_allowlist: cloneConfig(drafts[id]) })
        saved[id] = cloneConfig(updated.model_allowlist)
        drafts[id] = cloneConfig(updated.model_allowlist)
        const index = groups.value.findIndex(group => group.id === id)
        if (index >= 0) groups.value[index] = updated
        count++
      } catch (error) {
        const name = groups.value.find(group => group.id === id)?.name ?? String(id)
        failures.push(`${name}: ${errorText(error, t('admin.modelAllowlist.saveFailed'))}`)
      }
    }
    if (count) appStore.showSuccess(t('admin.modelAllowlist.saved', { count }))
    if (failures.length) appStore.showError(failures.join('\n'))
  } finally {
    saving.value = false
  }
}
function resolveLeave(leave: boolean) {
  leaveOpen.value = false
  leaveResolver?.(leave)
  leaveResolver = undefined
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirtyIds.value.length || saving.value) {
    event.preventDefault()
    event.returnValue = ''
  }
}
onBeforeRouteLeave(() => {
  if (saving.value) return false
  if (!dirtyIds.value.length) return true
  leaveOpen.value = true
  return new Promise<boolean>(resolve => { leaveResolver = resolve })
})
onMounted(() => {
  void loadGroups()
  window.addEventListener('beforeunload', beforeUnload)
})
onUnmounted(() => {
  window.removeEventListener('beforeunload', beforeUnload)
  leaveResolver?.(false)
})
</script>

<style scoped>
.allowlist-page { height: calc(100dvh - 64px - 2rem); }
@media (min-width: 768px) { .allowlist-page { height: calc(100dvh - 64px - 3rem); } }
@media (min-width: 1024px) { .allowlist-page { height: calc(100dvh - 64px - 4rem); } }
</style>
