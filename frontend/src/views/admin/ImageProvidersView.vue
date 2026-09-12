<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('imageProviders.hint') }}</p>
          <div class="flex gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
            <button class="btn btn-secondary" :disabled="loading || !group" @click="openCopy">{{ t('imageProviders.copyExisting') }}</button>
            <button class="btn btn-primary" :disabled="loading || !group" @click="openCreate">{{ t('imageProviders.create') }}</button>
          </div>
        </div>
        <div v-if="!loading && (!group || group.status !== 'active' || !group.allow_image_generation)" class="mt-3 flex items-center gap-3 rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
          <span>{{ t('imageProviders.groupUnavailable') }}</span>
          <RouterLink to="/admin/groups" class="underline">{{ t('nav.groups') }}</RouterLink>
        </div>
        <section class="relative mt-4 overflow-hidden rounded-2xl border border-primary-100 bg-white shadow-sm dark:border-primary-900/50 dark:bg-dark-800">
          <div class="absolute inset-y-0 left-0 w-1 bg-primary-500" aria-hidden="true"></div>
          <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="flex min-w-0 items-start gap-3">
                <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary-50 font-mono text-base font-semibold text-primary-700 ring-1 ring-inset ring-primary-100 dark:bg-primary-900/30 dark:text-primary-300 dark:ring-primary-800">
                  ↗
                </div>
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <h2 class="text-sm font-semibold text-gray-900 dark:text-dark-100">{{ t('imageProviders.codexMappingTitle') }}</h2>
                    <span :class="codexMapping.target_model.trim() ? 'bg-emerald-50 text-emerald-700 ring-emerald-600/20 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-100 text-gray-500 ring-gray-500/10 dark:bg-dark-700 dark:text-dark-300'" class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset">
                      <span :class="codexMapping.target_model.trim() ? 'bg-emerald-500' : 'bg-gray-400'" class="h-1.5 w-1.5 rounded-full"></span>
                      {{ codexMapping.target_model.trim() ? t('imageProviders.codexMappingActive') : t('imageProviders.codexMappingInactive') }}
                    </span>
                  </div>
                  <p class="mt-1 max-w-3xl text-sm leading-5 text-gray-500 dark:text-dark-400">{{ t('imageProviders.codexMappingHint') }}</p>
                </div>
              </div>
              <button class="btn btn-primary shrink-0" :disabled="codexMappingLoading || codexMappingSaving" @click="saveCodexMapping">
                {{ codexMappingSaving ? t('common.saving') : t('imageProviders.codexMappingSave') }}
              </button>
            </div>
          </div>

          <div class="grid items-stretch gap-3 px-5 py-5 md:grid-cols-[minmax(0,1fr)_5rem_minmax(0,1fr)]">
            <div class="rounded-xl border border-gray-200 bg-gray-50/70 p-3.5 dark:border-dark-600 dark:bg-dark-700/50">
              <div class="mb-2 flex items-center justify-between gap-3">
                <label for="codex-builtin-image-model" class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-300">{{ t('imageProviders.codexRequestModel') }}</label>
                <span class="rounded bg-white px-1.5 py-0.5 font-mono text-[10px] text-gray-400 ring-1 ring-inset ring-gray-200 dark:bg-dark-800 dark:ring-dark-600">CODEX</span>
              </div>
              <input id="codex-builtin-image-model" v-model="codexMapping.builtin_model" class="input border-0 bg-white font-mono shadow-sm ring-1 ring-inset ring-gray-200 focus:ring-2 focus:ring-primary-500 dark:bg-dark-800 dark:ring-dark-600" :disabled="codexMappingLoading" />
              <p class="mt-2 text-xs text-gray-400 dark:text-dark-400">{{ t('imageProviders.codexBuiltinModel') }}</p>
            </div>

            <div class="flex items-center justify-center" aria-hidden="true">
              <span class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-600 text-sm font-semibold text-white shadow-sm ring-4 ring-primary-50 dark:ring-primary-900/30 md:hidden">↓</span>
              <div class="hidden w-full items-center md:flex">
                <span class="h-px flex-1 bg-primary-200 dark:bg-primary-900"></span>
                <span class="mx-2 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-600 text-sm font-semibold text-white shadow-sm ring-4 ring-primary-50 dark:ring-primary-900/30">→</span>
                <span class="h-px flex-1 bg-primary-200 dark:bg-primary-900"></span>
              </div>
            </div>

            <div class="rounded-xl border border-primary-200 bg-primary-50/40 p-3.5 dark:border-primary-900/70 dark:bg-primary-900/10">
              <div class="mb-2 flex items-center justify-between gap-3">
                <label for="codex-upstream-image-model" class="text-xs font-semibold uppercase tracking-wider text-primary-700 dark:text-primary-300">{{ t('imageProviders.codexResolvedModel') }}</label>
                <span class="rounded bg-white px-1.5 py-0.5 font-mono text-[10px] text-primary-500 ring-1 ring-inset ring-primary-100 dark:bg-dark-800 dark:ring-primary-900">IMAGE GROUP</span>
              </div>
              <input id="codex-upstream-image-model" v-model="codexMapping.target_model" class="input border-0 bg-white font-mono shadow-sm ring-1 ring-inset ring-primary-200 focus:ring-2 focus:ring-primary-500 dark:bg-dark-800 dark:ring-primary-900" list="codex-image-target-options" :disabled="codexMappingLoading" :placeholder="t('imageProviders.codexTargetPlaceholder')" />
              <datalist id="codex-image-target-options">
                <option v-for="model in codexTargetOptions" :key="model" :value="model" />
              </datalist>
              <p class="mt-2 text-xs text-primary-600/70 dark:text-primary-300/70">{{ t('imageProviders.codexTargetModel') }}</p>
            </div>
          </div>
          <p v-if="codexMappingError" role="alert" class="mx-5 mb-5 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ codexMappingError }}</p>
        </section>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="accounts" :loading="loading">
          <template #cell-platform="{ row }">{{ accountTypeLabel(row.platform) }}</template>
          <template #cell-base_url="{ row }">
            <span class="block max-w-80 truncate" :title="String(row.credentials?.base_url || '')">{{ row.credentials?.base_url || '—' }}</span>
          </template>
          <template #cell-models="{ row }">
            <div class="flex max-w-lg flex-wrap gap-1">
              <span v-for="model in modelNames(row)" :key="model" class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">{{ model }}</span>
            </div>
          </template>
          <template #cell-status="{ row }">
            <span :class="row.status === 'active' && row.schedulable ? 'text-emerald-600' : 'text-gray-500'">{{ row.status === 'active' && row.schedulable ? t('common.enabled') : t('imageProviders.disabled') }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex gap-3">
              <button class="text-primary-600 hover:underline" :disabled="saving" @click="openEdit(row.id)">{{ t('common.edit') }}</button>
              <button class="text-gray-600 hover:underline dark:text-dark-300" :disabled="saving" @click="toggle(row)">{{ row.status === 'active' && row.schedulable ? t('imageProviders.disable') : t('imageProviders.enable') }}</button>
              <button class="text-red-600 hover:underline dark:text-red-400" :disabled="saving" @click="openDelete(row)">{{ t('common.delete') }}</button>
            </div>
          </template>
        </DataTable>
      </template>
      <template #pagination>
        <Pagination :page="page" :total="total" :page-size="100" :show-page-size-selector="false" @update:page="changePage" />
      </template>
    </TablePageLayout>
    <BaseDialog :show="showForm" :title="editing ? t('imageProviders.edit') : t('imageProviders.create')" width="normal" @close="closeForm">
      <form id="image-provider-form" class="space-y-5" @submit.prevent="save">
        <div>
          <label for="image-provider-name" class="input-label">{{ t('common.name') }}</label>
          <input id="image-provider-name" v-model="form.name" class="input" required maxlength="100" />
        </div>
        <div>
          <label class="input-label">{{ t('imageProviders.accountType') }}</label>
          <Select v-model="form.platform" :disabled="!!editing" :options="accountTypeOptions" />
          <p class="input-hint">{{ t('imageProviders.accountTypeHint') }}</p>
        </div>
        <div>
          <label for="image-provider-url" class="input-label">Base URL</label>
          <input id="image-provider-url" v-model="form.baseURL" class="input" type="url" required placeholder="https://api.example.com/v1" />
        </div>
        <div>
          <label for="image-provider-key" class="input-label">API Key</label>
          <input id="image-provider-key" v-model="form.apiKey" class="input font-mono" type="password" autocomplete="new-password" :required="!editing" :placeholder="editing ? t('imageProviders.keepKey') : 'sk-…'" />
        </div>
        <div>
          <div class="mb-2 flex items-center justify-between gap-3">
            <span id="image-provider-models-label" class="input-label mb-0">{{ t('imageProviders.models') }}</span>
            <button type="button" class="btn btn-secondary" :disabled="fetchingModels || saving || !canFetchModels" @click="fetchModels">
              {{ fetchingModels ? t('imageProviders.fetchingModels') : t('imageProviders.fetchModels') }}
            </button>
          </div>
          <p class="input-hint mb-2">{{ t('imageProviders.fetchModelsHint') }}</p>
          <p v-if="modelsError" role="alert" class="mb-2 text-sm text-red-600">{{ modelsError }}</p>
          <div role="group" aria-labelledby="image-provider-models-label" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" :aria-busy="fetchingModels">
            <div class="border-b border-gray-200 pb-3 dark:border-dark-600">
              <div class="mb-2 flex items-center justify-between gap-3">
                <p class="text-sm font-medium">{{ t('imageProviders.selectedModels', { count: selectedModels.length }) }}</p>
                <button
                  type="button"
                  class="text-sm text-gray-500 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-50 dark:text-dark-400 dark:hover:text-red-400"
                  :disabled="saving || fetchingModels || selectedModels.length === 0"
                  @click="clearSelectedModels"
                >
                  {{ t('imageProviders.clearSelectedModels') }}
                </button>
              </div>
              <div v-if="selectedModels.length" class="flex flex-wrap gap-2">
                <span v-for="model in selectedModels" :key="model.line" class="inline-flex max-w-full items-center gap-1 rounded-lg bg-primary-50 px-2 py-1 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">
                  <span class="break-all text-sm">{{ model.name }}<span v-if="model.name !== model.target" class="opacity-70"> → {{ model.target }}</span></span>
                  <button type="button" class="shrink-0 rounded px-1 hover:bg-primary-100 dark:hover:bg-primary-900/40" :aria-label="t('imageProviders.removeModel', { model: model.name })" @click="removeSelectedModel(model.line)">×</button>
                </span>
              </div>
              <p v-else class="text-sm text-gray-500 dark:text-dark-400">{{ t('imageProviders.noSelectedModels') }}</p>
            </div>
            <p v-if="fetchingModels" role="status" class="py-3 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('imageProviders.fetchingModels') }}</p>
            <template v-else-if="remoteModels !== null">
              <input v-model="modelSearch" class="input" type="search" :aria-label="t('imageProviders.searchModels')" :placeholder="t('imageProviders.searchModels')" />
              <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
                <label class="flex items-center gap-2">
                  <input v-model="imageModelsOnly" type="checkbox" class="rounded" />
                  {{ t('imageProviders.imageModelsOnly') }}
                </label>
                <button type="button" class="text-primary-600 hover:underline" :disabled="fetchingModels || !visibleRemoteModels.length" @click="selectVisibleModels">{{ t('imageProviders.selectVisibleModels') }}</button>
              </div>
              <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('imageProviders.remoteModelsCount', { count: visibleRemoteModels.length, total: remoteModels.length }) }}</p>
              <div class="max-h-48 space-y-1 overflow-y-auto">
                <label v-for="model in visibleRemoteModels" :key="model" class="flex cursor-pointer items-start gap-2 rounded px-2 py-1.5 hover:bg-gray-50 dark:hover:bg-dark-700">
                  <input type="checkbox" class="mt-0.5 rounded" :value="model" :checked="selectedModelTargets.has(model)" :disabled="fetchingModels" @change="toggleRemoteModel(model, ($event.target as HTMLInputElement).checked)" />
                  <span class="break-all font-mono text-sm">{{ model }}</span>
                </label>
                <p v-if="!visibleRemoteModels.length" class="py-2 text-sm text-gray-500">{{ remoteModels.length ? t('imageProviders.noMatchingModels') : t('imageProviders.noRemoteModels') }}</p>
              </div>
            </template>
            <p v-else class="py-3 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('imageProviders.fetchToSelect') }}</p>
          </div>
          <p class="input-hint">{{ t('imageProviders.groupModelsHint') }}</p>
          <button type="button" class="mt-3 text-sm text-gray-500 hover:text-primary-600 dark:text-dark-400" :aria-expanded="showAdvancedModels" aria-controls="image-provider-advanced-models" @click="showAdvancedModels = !showAdvancedModels">{{ t('imageProviders.advancedModels') }}</button>
          <div v-if="showAdvancedModels" id="image-provider-advanced-models" class="mt-3">
            <label for="image-provider-models" class="input-label">{{ t('imageProviders.manualModels') }}</label>
            <textarea id="image-provider-models" v-model="form.models" class="input min-h-28 font-mono" placeholder="gemini-3.1-flash-image&#10;gpt-image-2" />
            <p class="input-hint">{{ t('imageProviders.modelsHint') }}</p>
          </div>
        </div>
        <div>
          <label for="image-provider-concurrency" class="input-label">{{ t('imageProviders.concurrency') }}</label>
          <input id="image-provider-concurrency" v-model.number="form.concurrency" class="input" type="number" min="1" max="1000" required />
        </div>
        <p v-if="formError" role="alert" class="text-sm text-red-600">{{ formError }}</p>
      </form>
      <template #footer>
        <button class="btn btn-secondary" :disabled="saving" @click="closeForm">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" type="submit" form="image-provider-form" :disabled="saving">{{ saving ? t('common.saving') : t('common.save') }}</button>
      </template>
    </BaseDialog>
    <BaseDialog :show="showCopy" :title="t('imageProviders.copyExisting')" width="normal" @close="closeCopy">
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('imageProviders.copyHint') }}</p>
        <p v-if="copyLoading" role="status" class="py-3 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</p>
        <template v-else>
          <div v-if="copyAccountOptions.length">
            <label class="input-label">{{ t('imageProviders.sourceAccount') }}</label>
            <Select
              v-model="copySourceID"
              :options="copyAccountOptions"
              :placeholder="t('imageProviders.selectSourceAccount')"
              searchable
            />
          </div>
          <p v-else class="rounded-lg bg-gray-50 p-3 text-sm text-gray-500 dark:bg-dark-700 dark:text-dark-300">{{ t('imageProviders.noCopyCandidates') }}</p>
        </template>
        <p v-if="copyError" role="alert" class="text-sm text-red-600">{{ copyError }}</p>
      </div>
      <template #footer>
        <button class="btn btn-secondary" :disabled="copying" @click="closeCopy">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="copyLoading || copying || !copySourceID" @click="confirmCopy">
          {{ copying ? t('imageProviders.copying') : t('imageProviders.copyConfirm') }}
        </button>
      </template>
    </BaseDialog>
    <BaseDialog :show="!!deleteTarget" :title="t('imageProviders.deleteTitle')" width="narrow" @close="cancelDelete">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('imageProviders.deleteConfirm', { name: deleteTarget?.name }) }}</p>
      <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">{{ t('imageProviders.deleteImpact') }}</p>
      <p v-if="deleteError" role="alert" class="mt-3 text-sm text-red-600">{{ deleteError }}</p>
      <template #footer>
        <button class="btn btn-secondary" :disabled="saving" @click="cancelDelete">{{ t('common.cancel') }}</button>
        <button class="btn btn-danger" :disabled="saving" @click="confirmDelete">{{ saving ? t('imageProviders.deleting') : t('common.delete') }}</button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { Account, AccountListItem, AdminGroup } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'

const { t } = useI18n()
const app = useAppStore()
const accounts = ref<AccountListItem[]>([])
const group = ref<AdminGroup | null>(null)
const loading = ref(false)
const saving = ref(false)
const page = ref(1)
const total = ref(0)
const showForm = ref(false)
const editing = ref<Account | null>(null)
const deleteTarget = ref<AccountListItem | null>(null)
const deleteError = ref('')
const formError = ref('')
const showCopy = ref(false)
const copyLoading = ref(false)
const copying = ref(false)
const copyCandidates = ref<AccountListItem[]>([])
const copySourceID = ref<number | null>(null)
const copyError = ref('')
const codexMapping = ref({ builtin_model: 'gpt-image-2', target_model: '' })
const codexMappingLoading = ref(false)
const codexMappingSaving = ref(false)
const codexMappingError = ref('')
type ImageAccountPlatform = 'openai' | 'gemini' | 'grok'

const emptyForm = () => ({ name: '', platform: 'openai' as ImageAccountPlatform, baseURL: '', apiKey: '', models: '', concurrency: 3 })
const form = ref(emptyForm())
const fetchingModels = ref(false)
const remoteModels = ref<string[] | null>(null)
const modelsError = ref('')
const modelSearch = ref('')
const imageModelsOnly = ref(true)
const showAdvancedModels = ref(false)
let modelsRequest = 0
const canFetchModels = computed(() => !!form.value.baseURL.trim() && (!!editing.value || !!form.value.apiKey.trim()))
const visibleRemoteModels = computed(() => (remoteModels.value || []).filter(model =>
  (!imageModelsOnly.value || /image|dall-e|imagen|banana|flux|stable-diffusion|sdxl/i.test(model)) &&
  model.toLowerCase().includes(modelSearch.value.trim().toLowerCase())
))
const selectedModelTargets = computed(() => new Set(modelLines().map(modelTarget)))
const selectedModels = computed(() => [...new Set(modelLines())].map(line => ({ line, name: line.split('=')[0].trim(), target: modelTarget(line) })))
const codexTargetOptions = computed(() => [...new Set(accounts.value.flatMap(modelNames))].sort())

function modelLines() { return form.value.models.split('\n').map(line => line.trim()).filter(Boolean) }
function modelTarget(line: string) { return line.includes('=') ? line.slice(line.indexOf('=') + 1).trim() : line }
function removeSelectedModel(line: string) { form.value.models = modelLines().filter(value => value !== line).join('\n') }
function clearSelectedModels() { form.value.models = '' }

function toggleRemoteModel(model: string, checked: boolean) {
  const lines = modelLines()
  if (checked) {
    if (!selectedModelTargets.value.has(model)) lines.push(model)
    form.value.models = lines.join('\n')
  } else {
    form.value.models = lines.filter(line => modelTarget(line) !== model).join('\n')
  }
}

function selectVisibleModels() {
  form.value.models = [...modelLines(), ...visibleRemoteModels.value.filter(model => !selectedModelTargets.value.has(model))].join('\n')
}

function resetRemoteModels() {
  modelsRequest++
  fetchingModels.value = false
  remoteModels.value = null
  modelsError.value = ''
  modelSearch.value = ''
  imageModelsOnly.value = true
}

watch(() => [showForm.value, editing.value?.id, form.value.platform, form.value.baseURL, form.value.apiKey], resetRemoteModels)

async function fetchModels() {
  if (fetchingModels.value || !canFetchModels.value) return
  const request = ++modelsRequest
  fetchingModels.value = true
  modelsError.value = ''
  try {
    const result = await adminAPI.accounts.previewImageModels({
      account_id: editing.value?.id,
      platform: form.value.platform,
      base_url: form.value.baseURL.trim(),
      ...(form.value.apiKey.trim() ? { api_key: form.value.apiKey.trim() } : {})
    })
    if (request !== modelsRequest) return
    remoteModels.value = [...new Set(result.models.map(model => model.trim()).filter(Boolean))].sort()
  } catch (error) {
    if (request === modelsRequest) modelsError.value = message(error)
  } finally {
    if (request === modelsRequest) fetchingModels.value = false
  }
}
const accountTypeOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'grok', label: 'Grok / xAI' }
]
const copyAccountOptions = computed(() => copyCandidates.value.map(account => ({
  value: account.id,
  label: `${account.name} · ${accountTypeLabel(account.platform)}`,
  description: String(account.credentials?.base_url || '')
})))
const columns = computed(() => [
  { key: 'name', label: t('common.name') },
  { key: 'platform', label: t('imageProviders.accountType') },
  { key: 'base_url', label: 'Base URL' },
  { key: 'models', label: t('imageProviders.models') },
  { key: 'status', label: t('common.status') },
  { key: 'actions', label: t('common.actions') }
])

function accountTypeLabel(platform: string) {
  return accountTypeOptions.find(option => option.value === platform)?.label || platform
}

function modelNames(account: AccountListItem) {
  return Object.keys((account.credentials?.model_mapping || {}) as Record<string, string>)
}

function message(error: unknown) {
  const detail = error as { message?: string; response?: { data?: { message?: string; detail?: string } } }
  const response = detail?.response?.data
  return response?.detail || response?.message || detail?.message || t('imageProviders.failed')
}

async function load() {
  loading.value = true
  try {
    const groups = await adminAPI.groups.getAllIncludingInactive()
    group.value = groups.find(item => item.system_role === 'image_generation') || null
    if (!group.value) {
      accounts.value = []
      total.value = 0
      return
    }
    const result = await adminAPI.accounts.list(page.value, 100, { group: String(group.value.id), type: 'apikey', lite: 'false' })
    accounts.value = result.items.filter(item => item.purpose === 'image_provider')
    total.value = result.total
  } catch (error) {
    app.showError(message(error))
  } finally {
    loading.value = false
  }
}

async function loadCodexMapping() {
  codexMappingLoading.value = true
  codexMappingError.value = ''
  try {
    codexMapping.value = await adminAPI.settings.getCodexImageModelMappingSettings()
  } catch (error) {
    codexMappingError.value = message(error)
  } finally {
    codexMappingLoading.value = false
  }
}

async function saveCodexMapping() {
  if (codexMappingSaving.value) return
  codexMappingSaving.value = true
  codexMappingError.value = ''
  try {
    codexMapping.value = await adminAPI.settings.updateCodexImageModelMappingSettings({
      builtin_model: codexMapping.value.builtin_model.trim(),
      target_model: codexMapping.value.target_model.trim()
    })
    app.showSuccess(t('imageProviders.codexMappingSaved'))
  } catch (error) {
    codexMappingError.value = message(error)
  } finally {
    codexMappingSaving.value = false
  }
}

function openCreate() {
  editing.value = null
  form.value = emptyForm()
  formError.value = ''
  showAdvancedModels.value = false
  showForm.value = true
}

async function loadCopyCandidates() {
  copyLoading.value = true
  copyError.value = ''
  try {
    const candidates: AccountListItem[] = []
    let sourcePage = 1
    let sourceTotal = 0
    do {
      const result = await adminAPI.accounts.list(sourcePage, 100, { type: 'apikey', lite: 'false' })
      candidates.push(...result.items)
      sourceTotal = result.total
      sourcePage++
      if (!result.items.length) break
    } while (candidates.length < sourceTotal)
    copyCandidates.value = candidates.filter(account =>
      account.purpose !== 'image_provider' && (account.platform === 'openai' || account.platform === 'gemini' || account.platform === 'grok')
    )
    copySourceID.value = copyCandidates.value[0]?.id || null
  } catch (error) {
    copyCandidates.value = []
    copySourceID.value = null
    copyError.value = message(error)
  } finally {
    copyLoading.value = false
  }
}

function openCopy() {
  copyCandidates.value = []
  copySourceID.value = null
  copyError.value = ''
  showCopy.value = true
  void loadCopyCandidates()
}

function closeCopy() {
  if (copying.value) return
  showCopy.value = false
  copyError.value = ''
}

async function confirmCopy() {
  if (!group.value || !copySourceID.value || copying.value) return
  copying.value = true
  copyError.value = ''
  try {
    const copied = await adminAPI.accounts.copyToImageProvider(copySourceID.value, group.value.id)
    showCopy.value = false
    app.showSuccess(t('imageProviders.copySuccess', { name: copied.name }))
    await load()
  } catch (error) {
    copyError.value = message(error)
  } finally {
    copying.value = false
  }
}

async function openEdit(id: number) {
  saving.value = true
  try {
    const account = await adminAPI.accounts.getById(id)
    editing.value = account
    const mapping = (account.credentials?.model_mapping || {}) as Record<string, string>
    form.value = {
      name: account.name,
      platform: account.platform as ImageAccountPlatform,
      baseURL: String(account.credentials?.base_url || ''),
      apiKey: '',
      models: Object.entries(mapping).map(([model, target]) => model === target ? model : `${model}=${target}`).join('\n'),
      concurrency: account.concurrency
    }
    formError.value = ''
    showAdvancedModels.value = false
    showForm.value = true
    await nextTick()
    void fetchModels()
  } catch (error) {
    app.showError(message(error))
  } finally {
    saving.value = false
  }
}

function closeForm() {
  if (saving.value) return
  showForm.value = false
  form.value.apiKey = ''
}

async function save() {
  if (!group.value) return
  const mapping: Record<string, string> = {}
  for (const line of form.value.models.split('\n').map(value => value.trim()).filter(Boolean)) {
    const [model, ...targetParts] = line.split('=')
    const name = model.trim()
    const target = targetParts.length ? targetParts.join('=').trim() : name
    if (!name || !target || name.includes('*')) {
      formError.value = t('imageProviders.invalidModels')
      return
    }
    mapping[name] = target
  }
  saving.value = true
  formError.value = ''
  try {
    const credentials: Record<string, unknown> = { ...editing.value?.credentials, base_url: form.value.baseURL.trim(), model_mapping: mapping }
    if (form.value.apiKey.trim()) credentials.api_key = form.value.apiKey.trim()
    if (editing.value) {
      await adminAPI.accounts.update(editing.value.id, { name: form.value.name.trim(), credentials, concurrency: form.value.concurrency })
    } else {
      await adminAPI.accounts.create({ name: form.value.name.trim(), platform: form.value.platform, type: 'apikey', purpose: 'image_provider', credentials,
        group_ids: [group.value.id], concurrency: form.value.concurrency, priority: 1 })
    }
    showForm.value = false
    form.value.apiKey = ''
    app.showSuccess(t('imageProviders.saved'))
    await load()
  } catch (error) {
    formError.value = message(error)
  } finally {
    saving.value = false
  }
}

async function toggle(account: AccountListItem) {
  saving.value = true
  try {
    const enabled = !(account.status === 'active' && account.schedulable)
    if (enabled && account.status !== 'active') {
      await adminAPI.accounts.update(account.id, { status: 'active' })
    }
    await adminAPI.accounts.setSchedulable(account.id, enabled)
    await load()
  } catch (error) {
    app.showError(message(error))
  } finally {
    saving.value = false
  }
}

function openDelete(account: AccountListItem) {
  deleteTarget.value = account
  deleteError.value = ''
}

function cancelDelete() {
  if (saving.value) return
  deleteTarget.value = null
  deleteError.value = ''
}

async function confirmDelete() {
  if (!deleteTarget.value || saving.value) return
  saving.value = true
  deleteError.value = ''
  try {
    await adminAPI.accounts.delete(deleteTarget.value.id)
    deleteTarget.value = null
    if (accounts.value.length === 1 && page.value > 1) page.value--
    app.showSuccess(t('imageProviders.deleted'))
    await load()
  } catch (error) {
    deleteError.value = message(error)
  } finally {
    saving.value = false
  }
}

function changePage(value: number) { page.value = value; void load() }
onMounted(() => {
  void load()
  void loadCodexMapping()
})
</script>
