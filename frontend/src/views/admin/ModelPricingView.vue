<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-4">
          <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
            <div>
              <div class="flex items-center gap-2">
                <span class="inline-flex h-9 w-9 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
                  <Icon name="calculator" size="md" />
                </span>
                <div>
                  <h1 class="text-xl font-semibold text-gray-950 dark:text-white">
                    {{ t('admin.modelPricing.title') }}
                  </h1>
                  <p class="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.modelPricing.description') }}
                  </p>
                </div>
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <button class="btn btn-secondary" :disabled="loading || refreshing" @click="loadCatalog">
                <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
                <span class="ml-2">{{ t('common.refresh') }}</span>
              </button>
              <button class="btn btn-secondary" :disabled="loading || refreshing" @click="refreshRemote">
                <Icon name="cloud" size="sm" :class="refreshing ? 'animate-pulse' : ''" />
                <span class="ml-2">{{ t('admin.modelPricing.refreshRemote') }}</span>
              </button>
              <button class="btn btn-primary" @click="openCreate">
                <Icon name="plus" size="sm" />
                <span class="ml-2">{{ t('admin.modelPricing.addRule') }}</span>
              </button>
            </div>
          </div>

          <div class="pricing-ledger-grid">
            <div class="ledger-stat">
              <span class="ledger-label">{{ t('admin.modelPricing.enabledGroupModels') }}</span>
              <strong>{{ groupPricingEntries.length }}</strong>
              <span>{{ t('admin.modelPricing.groupCoverage', {
                groups: activeGroups.length,
                catalog: catalog?.model_count ?? 0
              }) }}</span>
            </div>
            <div class="ledger-stat ledger-stat-accent">
              <span class="ledger-label">{{ t('admin.modelPricing.localOverrides') }}</span>
              <strong>{{ catalog?.override_count ?? 0 }}</strong>
              <span>{{ t('admin.modelPricing.hotReloaded') }}</span>
            </div>
            <div class="ledger-stat" :class="{ 'ledger-stat-warning': unpricedCount > 0 }">
              <span class="ledger-label">{{ t('admin.modelPricing.unpricedModels') }}</span>
              <strong>{{ unpricedCount }}</strong>
              <span>{{ unpricedCount ? t('admin.modelPricing.needsAttention') : t('admin.modelPricing.allPriced') }}</span>
            </div>
            <div class="ledger-file">
              <span class="ledger-label">{{ t('admin.modelPricing.overrideFile') }}</span>
              <code :title="catalog?.override_file">{{ catalog?.override_file || '—' }}</code>
              <span>{{ t('admin.modelPricing.sharedVolumeHint') }}</span>
            </div>
          </div>

          <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
            <div class="relative min-w-0 flex-1 lg:max-w-md">
              <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="search"
                class="input pl-9"
                type="search"
                :placeholder="t('admin.modelPricing.searchPlaceholder')"
              />
            </div>
            <Select v-model="scopeFilter" class="w-full lg:w-64" :options="scopeOptions" />
            <Select v-model="sourceFilter" class="w-full lg:w-44" :options="sourceOptions" />
            <Select v-model="modeFilter" class="w-full lg:w-44" :options="modeOptions" />
            <div class="ml-auto text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.modelPricing.updatedAt') }}：{{ formatDate(catalog?.last_updated) }}
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="pagedItems" :loading="loading">
          <template #cell-model="{ row }">
            <div class="min-w-[220px]">
              <div class="flex items-center gap-2">
                <code class="font-semibold text-gray-900 dark:text-gray-100">{{ row.model }}</code>
                <span v-if="row.wildcard" class="rule-badge">{{ t('admin.modelPricing.prefixRule') }}</span>
                <span v-if="row.inherited_from" class="rule-badge">{{ t('admin.modelPricing.inheritedRule', { rule: row.inherited_from }) }}</span>
                <span v-if="row.deprecated" class="deprecated-badge">{{ t('admin.modelPricing.deprecated') }}</span>
              </div>
              <div class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                <span>{{ row.litellm_provider || t('admin.modelPricing.unknownProvider') }}</span>
                <span>·</span>
                <span>{{ row.mode || 'chat' }}</span>
                <span>·</span>
                <span class="font-semibold">{{ resolvePricingCurrency(row.litellm_provider, row.model) }}</span>
              </div>
              <div v-if="row.deprecation_date" class="mt-1 text-xs" :class="row.deprecated ? 'text-red-500 dark:text-red-400' : 'text-gray-400 dark:text-gray-500'">
                {{ t('admin.modelPricing.deprecationDate', { date: row.deprecation_date }) }}
              </div>
              <div v-if="row.group_names?.length" class="mt-1 text-xs text-primary-600 dark:text-primary-400">
                {{ t('admin.modelPricing.usedByGroups', { groups: row.group_names.join('、') }) }}
              </div>
            </div>
          </template>

          <template #cell-input_cost_per_token="{ row }">
            <PriceCell :value="row.input_cost_per_token" :provider="row.litellm_provider" :model="row.model" :missing="isPricingMissing(row)" />
          </template>
          <template #cell-output_cost_per_token="{ row }">
            <PriceCell :value="row.output_cost_per_token" :provider="row.litellm_provider" :model="row.model" :missing="isPricingMissing(row)" />
          </template>
          <template #cell-cache_creation_input_token_cost="{ row }">
            <PriceCell :value="row.cache_creation_input_token_cost" :provider="row.litellm_provider" :model="row.model" />
          </template>
          <template #cell-cache_read_input_token_cost="{ row }">
            <PriceCell :value="row.cache_read_input_token_cost" :provider="row.litellm_provider" :model="row.model" />
          </template>
          <template #cell-output_cost_per_image="{ row }">
            <span class="font-mono text-sm tabular-nums text-gray-700 dark:text-gray-300">
              {{ formatMediaPrice(row.output_cost_per_image, row) }}
            </span>
          </template>
          <template #cell-output_cost_per_video="{ row }">
            <span class="font-mono text-sm tabular-nums text-gray-700 dark:text-gray-300">
              {{ formatMediaPrice(row.output_cost_per_video, row) }}
            </span>
          </template>
          <template #cell-source="{ row }">
            <div class="source-track" :class="sourceTrackClass(row)">
              <span class="source-dot"></span>
              <span>{{ pricingSourceLabel(row) }}</span>
            </div>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end gap-1">
              <button class="icon-action" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button
                v-if="row.overridden && !row.inherited_from"
                class="icon-action icon-action-danger"
                :title="t('admin.modelPricing.restoreCatalog')"
                @click="deleteTarget = row"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :total="filteredItems.length"
          :page="page"
          :page-size="pageSize"
          @update:page="page = $event"
          @update:page-size="changePageSize"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showEditor" :title="editing ? t('admin.modelPricing.editRule') : t('admin.modelPricing.addRule')" width="wide" @close="closeEditor">
      <div class="space-y-6">
        <div class="rounded-xl border border-primary-100 bg-primary-50/60 p-4 text-sm text-primary-900 dark:border-primary-900/50 dark:bg-primary-900/20 dark:text-primary-200">
          <div class="flex gap-3">
            <Icon name="bolt" size="sm" class="mt-0.5 flex-shrink-0" />
            <p>{{ t('admin.modelPricing.editorHint') }}</p>
          </div>
        </div>

        <div class="grid gap-4 md:grid-cols-3">
          <label class="md:col-span-2">
            <span class="input-label">{{ t('admin.modelPricing.modelOrPrefix') }}</span>
            <input v-model="form.model" class="input font-mono" :disabled="Boolean(editing)" placeholder="qwen3-*" />
          </label>
          <label>
            <span class="input-label">{{ t('admin.modelPricing.mode') }}</span>
            <input v-model="form.mode" class="input" placeholder="chat" />
          </label>
          <label class="md:col-span-2">
            <span class="input-label">{{ t('admin.modelPricing.provider') }}</span>
            <input v-model="form.provider" class="input" placeholder="dashscope" />
          </label>
          <div class="flex items-end pb-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.modelPricing.wildcardHint') }}
          </div>
        </div>

        <div>
          <div class="mb-3 flex items-end justify-between gap-4 border-b border-gray-200 pb-2 dark:border-dark-700">
            <div>
              <h4 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.modelPricing.tokenPrices') }}</h4>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.modelPricing.priceInputHint', { currency: editorCurrency }) }}
              </p>
            </div>
            <span class="unit-chip">{{ editorCurrency }} / 1M tokens</span>
          </div>
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <label v-for="field in tokenPriceFields" :key="field.key">
              <span class="input-label">{{ t(field.label) }}</span>
              <input v-model="form.prices[field.key]" class="input font-mono tabular-nums" type="number" min="0" step="any" placeholder="—" />
            </label>
          </div>
        </div>

        <details class="group rounded-xl border border-gray-200 dark:border-dark-700">
          <summary class="flex cursor-pointer list-none items-center justify-between px-4 py-3 text-sm font-medium text-gray-800 dark:text-gray-200">
            <span>{{ t('admin.modelPricing.advancedPrices') }}</span>
            <Icon name="chevronDown" size="sm" class="transition-transform group-open:rotate-180" />
          </summary>
          <div class="grid gap-4 border-t border-gray-200 p-4 sm:grid-cols-2 lg:grid-cols-3 dark:border-dark-700">
            <label v-for="field in advancedPriceFields" :key="field.key">
              <span class="input-label">{{ t(field.label) }}</span>
              <div class="relative">
                <input v-model="form.prices[field.key]" class="input font-mono tabular-nums" type="number" min="0" step="any" placeholder="—" />
                <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-400">{{ editorCurrencySymbol }}{{ field.unit }}</span>
              </div>
            </label>
          </div>
        </details>

        <p v-if="formError" class="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
          {{ formError }}
        </p>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" :disabled="saving" @click="closeEditor">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="saveRule">
            <Icon v-if="saving" name="refresh" size="sm" class="mr-2 animate-spin" />
            {{ t('admin.modelPricing.saveAndApply') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(deleteTarget)"
      :title="t('admin.modelPricing.restoreCatalog')"
      :message="t('admin.modelPricing.restoreConfirm', { model: deleteTarget?.model || '' })"
      :confirm-text="t('admin.modelPricing.restoreAction')"
      danger
      @confirm="removeOverride"
      @cancel="deleteTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import channelsAPI, { type ModelPricingCatalog, type ModelPricingCatalogEntry } from '@/api/admin/channels'
import groupsAPI from '@/api/admin/groups'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  displayToStoredPrice,
  pricingCurrencySymbol,
  resolvePricingCurrency,
  storedPriceToDisplay,
  type PricingCurrency
} from '@/utils/pricingCurrency'
import {
  filterPricingEntriesByScope,
  materializeGroupPricingEntries,
  type ModelPricingScope,
  type PricingGroupModels,
  type ScopedModelPricingEntry
} from '@/utils/modelPricingGroupScope'
import type { AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'

const { t, locale } = useI18n()
const appStore = useAppStore()

const PriceCell = defineComponent({
  props: {
    value: { type: Number, required: true },
    provider: { type: String, default: '' },
    model: { type: String, default: '' },
    missing: { type: Boolean, default: false }
  },
  setup(props) {
    return () => {
      const currency = resolvePricingCurrency(props.provider, props.model)
      const displayValue = storedPriceToDisplay(props.value, 1_000_000)
      return h('span', {
        class: props.missing
          ? 'font-mono text-sm font-semibold tabular-nums text-amber-600 dark:text-amber-400'
          : 'font-mono text-sm tabular-nums text-gray-700 dark:text-gray-300'
      }, `${pricingCurrencySymbol(currency)}${formatNumber(displayValue)}`)
    }
  }
})

type PriceKey =
  | 'input_cost_per_token'
  | 'output_cost_per_token'
  | 'cache_creation_input_token_cost'
  | 'cache_creation_input_token_cost_above_1hr'
  | 'cache_read_input_token_cost'
  | 'input_cost_per_token_priority'
  | 'output_cost_per_token_priority'
  | 'cache_creation_input_token_cost_priority'
  | 'cache_read_input_token_cost_priority'
  | 'output_cost_per_image'
  | 'output_cost_per_video'
  | 'input_cost_per_image_token'
  | 'output_cost_per_image_token'

interface PriceField { key: PriceKey; label: string; scale: number; unit: string }

const tokenPriceFields: PriceField[] = [
  { key: 'input_cost_per_token', label: 'admin.modelPricing.fields.input', scale: 1_000_000, unit: '/ 1M' },
  { key: 'output_cost_per_token', label: 'admin.modelPricing.fields.output', scale: 1_000_000, unit: '/ 1M' },
  { key: 'cache_creation_input_token_cost', label: 'admin.modelPricing.fields.cacheWrite', scale: 1_000_000, unit: '/ 1M' },
  { key: 'cache_creation_input_token_cost_above_1hr', label: 'admin.modelPricing.fields.cacheWrite1h', scale: 1_000_000, unit: '/ 1M' },
  { key: 'cache_read_input_token_cost', label: 'admin.modelPricing.fields.cacheRead', scale: 1_000_000, unit: '/ 1M' }
]

const advancedPriceFields: PriceField[] = [
  { key: 'input_cost_per_token_priority', label: 'admin.modelPricing.fields.priorityInput', scale: 1_000_000, unit: '/ 1M' },
  { key: 'output_cost_per_token_priority', label: 'admin.modelPricing.fields.priorityOutput', scale: 1_000_000, unit: '/ 1M' },
  { key: 'cache_creation_input_token_cost_priority', label: 'admin.modelPricing.fields.priorityCacheWrite', scale: 1_000_000, unit: '/ 1M' },
  { key: 'cache_read_input_token_cost_priority', label: 'admin.modelPricing.fields.priorityCacheRead', scale: 1_000_000, unit: '/ 1M' },
  { key: 'output_cost_per_image', label: 'admin.modelPricing.fields.perImage', scale: 1, unit: '/ image' },
  { key: 'output_cost_per_video', label: 'admin.modelPricing.fields.perVideo', scale: 1, unit: '/ video' },
  { key: 'input_cost_per_image_token', label: 'admin.modelPricing.fields.imageInputToken', scale: 1_000_000, unit: '/ 1M' },
  { key: 'output_cost_per_image_token', label: 'admin.modelPricing.fields.imageOutputToken', scale: 1_000_000, unit: '/ 1M' }
]
const allPriceFields = [...tokenPriceFields, ...advancedPriceFields]

const catalog = ref<ModelPricingCatalog | null>(null)
const activeGroups = ref<AdminGroup[]>([])
const groupModels = ref<PricingGroupModels[]>([])
const failedGroupCount = ref(0)
const loading = ref(false)
const refreshing = ref(false)
const saving = ref(false)
const search = ref('')
const scopeFilter = ref<ModelPricingScope>('active-groups')
const sourceFilter = ref<string | number | boolean | null>('all')
const modeFilter = ref<string | number | boolean | null>('all')
const page = ref(1)
const pageSize = ref(20)
const showEditor = ref(false)
const editing = ref<ModelPricingCatalogEntry | null>(null)
const deleteTarget = ref<ModelPricingCatalogEntry | null>(null)
const formError = ref('')
const form = reactive({ model: '', provider: '', mode: 'chat', prices: {} as Record<PriceKey, string> })
const editorCurrency = computed<PricingCurrency>(() => resolvePricingCurrency(form.provider, form.model))
const editorCurrencySymbol = computed(() => pricingCurrencySymbol(editorCurrency.value))

const columns = computed<Column[]>(() => [
  { key: 'model', label: t('admin.modelPricing.columns.model') },
  { key: 'input_cost_per_token', label: t('admin.modelPricing.columns.input') },
  { key: 'output_cost_per_token', label: t('admin.modelPricing.columns.output') },
  { key: 'cache_creation_input_token_cost', label: t('admin.modelPricing.columns.cacheWrite') },
  { key: 'cache_read_input_token_cost', label: t('admin.modelPricing.columns.cacheRead') },
  { key: 'output_cost_per_image', label: t('admin.modelPricing.columns.image') },
  { key: 'output_cost_per_video', label: t('admin.modelPricing.columns.video') },
  { key: 'source', label: t('admin.modelPricing.columns.source') },
  { key: 'actions', label: t('common.actions'), class: 'sticky right-0 bg-white dark:bg-dark-800' }
])

const sourceOptions = computed(() => [
  { value: 'all', label: t('admin.modelPricing.filters.allSources') },
  { value: 'override', label: t('admin.modelPricing.sourceOverride') },
  { value: 'catalog', label: t('admin.modelPricing.sourceCatalog') }
])
const scopeOptions = computed(() => [
  { value: 'active-groups', label: t('admin.modelPricing.filters.activeGroups') },
  ...activeGroups.value.map(group => ({
    value: `group:${group.id}`,
    label: t('admin.modelPricing.filters.specificGroup', { group: group.name })
  })),
  { value: 'catalog', label: t('admin.modelPricing.filters.fullCatalog') },
  { value: 'unused', label: t('admin.modelPricing.filters.unusedCatalog') }
])
const groupPricingEntries = computed(() => materializeGroupPricingEntries(catalog.value?.items || [], groupModels.value))
const scopedItems = computed(() => filterPricingEntriesByScope(
  catalog.value?.items || [],
  groupPricingEntries.value,
  scopeFilter.value
))
const modeOptions = computed(() => {
  const modes = [...new Set(scopedItems.value.map(item => item.mode || 'chat'))].sort()
  return [{ value: 'all', label: t('admin.modelPricing.filters.allModes') }, ...modes.map(value => ({ value, label: value }))]
})

const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return scopedItems.value.filter(item => {
    const usesRule = item.overridden || Boolean(item.inherited_from)
    if (sourceFilter.value === 'override' && !usesRule) return false
    if (sourceFilter.value === 'catalog' && usesRule) return false
    if (modeFilter.value !== 'all' && (item.mode || 'chat') !== modeFilter.value) return false
    if (!query) return true
    return `${item.model} ${item.litellm_provider} ${item.mode}`.toLowerCase().includes(query)
  })
})
const pagedItems = computed(() => filteredItems.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const unpricedCount = computed(() => groupPricingEntries.value.filter(isPricingMissing).length)

watch([search, scopeFilter, sourceFilter, modeFilter], () => { page.value = 1 })

function isPricingMissing(item: ModelPricingCatalogEntry) {
  if (item.mode === 'video') return item.output_cost_per_video <= 0
  if (['image_generation', 'image'].includes(item.mode)) {
    return item.output_cost_per_image <= 0 && item.input_cost_per_image_token <= 0 && item.output_cost_per_image_token <= 0
  }
  if (item.mode === 'audio') return false
  return item.token_pricing_absent || (item.input_cost_per_token === 0 && item.output_cost_per_token === 0)
}

function pricingSourceLabel(item: ScopedModelPricingEntry) {
  if (isPricingMissing(item)) return t('admin.modelPricing.sourceMissing')
  if (item.inherited_from) return t('admin.modelPricing.sourceInherited')
  return item.overridden ? t('admin.modelPricing.sourceOverride') : t('admin.modelPricing.sourceCatalog')
}

function sourceTrackClass(item: ScopedModelPricingEntry) {
  if (isPricingMissing(item)) return 'source-track-missing'
  if (item.inherited_from || item.overridden) return 'source-track-override'
  return 'source-track-catalog'
}

function formatNumber(value: number) {
  return new Intl.NumberFormat(locale.value, { minimumFractionDigits: 0, maximumFractionDigits: 6 }).format(value)
}

function formatMediaPrice(value: number, item: ModelPricingCatalogEntry) {
  if (value === 0) return '—'
  const currency = resolvePricingCurrency(item.litellm_provider, item.model)
  const displayValue = storedPriceToDisplay(value, 1)
  return `${pricingCurrencySymbol(currency)}${formatNumber(displayValue)}`
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString(locale.value)
}

function resetForm() {
  form.model = ''
  form.provider = ''
  form.mode = 'chat'
  form.prices = Object.fromEntries(allPriceFields.map(field => [field.key, ''])) as Record<PriceKey, string>
  formError.value = ''
}

function openCreate() {
  editing.value = null
  resetForm()
  showEditor.value = true
}

function openEdit(item: ModelPricingCatalogEntry) {
  resetForm()
  editing.value = item
  form.model = item.model
  form.provider = item.litellm_provider
  form.mode = item.mode || 'chat'
  for (const field of allPriceFields) {
    const value = item[field.key]
    const explicitlyOverridden = Object.prototype.hasOwnProperty.call(item.override || {}, field.key)
    form.prices[field.key] = value !== 0 || explicitlyOverridden
      ? String(storedPriceToDisplay(value, field.scale))
      : ''
  }
  showEditor.value = true
}

function closeEditor() {
  if (saving.value) return
  showEditor.value = false
  editing.value = null
  formError.value = ''
}

function buildPatch() {
  const patch: Record<string, unknown> = {}
  const previous = editing.value?.override || {}
  const provider = form.provider.trim()
  const mode = form.mode.trim()
  if (provider) patch.litellm_provider = provider
  else if (Object.prototype.hasOwnProperty.call(previous, 'litellm_provider')) patch.litellm_provider = null
  if (mode) patch.mode = mode
  else if (Object.prototype.hasOwnProperty.call(previous, 'mode')) patch.mode = null

  let hasPrice = false
  for (const field of allPriceFields) {
    const raw = form.prices[field.key].trim()
    if (raw === '') {
      if (Object.prototype.hasOwnProperty.call(previous, field.key)) patch[field.key] = null
      continue
    }
    const value = Number(raw)
    if (!Number.isFinite(value) || value < 0) throw new Error(t('admin.modelPricing.invalidPrice'))
    patch[field.key] = displayToStoredPrice(value, field.scale)
    hasPrice = true
  }
  if (!hasPrice) throw new Error(t('admin.modelPricing.priceRequired'))
  return patch
}

async function loadCatalog() {
  loading.value = true
  try {
    const [nextCatalog, groups] = await Promise.all([
      channelsAPI.listModelPricingCatalog(),
      groupsAPI.getAll()
    ])
    const candidateResults = await Promise.allSettled(groups.map(group =>
      groupsAPI.getModelAllowlistCandidates(group.id, group.platform)
    ))
    catalog.value = nextCatalog
    activeGroups.value = groups
    failedGroupCount.value = candidateResults.filter(result => result.status === 'rejected').length
    groupModels.value = groups.map((group, index) => ({
      group,
      candidates: candidateResults[index].status === 'fulfilled' ? candidateResults[index].value : []
    }))
    if (failedGroupCount.value > 0) {
      appStore.showError(t('admin.modelPricing.groupModelsLoadPartial', { count: failedGroupCount.value }))
    }
    const maxPage = Math.max(1, Math.ceil(filteredItems.value.length / pageSize.value))
    if (page.value > maxPage) page.value = maxPage
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelPricing.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function refreshRemote() {
  refreshing.value = true
  try {
    await channelsAPI.refreshModelPricingCatalog()
    await loadCatalog()
    appStore.showSuccess(t('admin.modelPricing.refreshSuccess'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelPricing.refreshFailed')))
  } finally {
    refreshing.value = false
  }
}

async function saveRule() {
  const model = form.model.trim()
  if (!model) {
    formError.value = t('admin.modelPricing.modelRequired')
    return
  }
  saving.value = true
  formError.value = ''
  try {
    await channelsAPI.saveModelPricingOverride(model, buildPatch())
    showEditor.value = false
    editing.value = null
    appStore.showSuccess(t('admin.modelPricing.saveSuccess'))
    await loadCatalog()
  } catch (error) {
    formError.value = extractApiErrorMessage(error, t('admin.modelPricing.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function removeOverride() {
  if (!deleteTarget.value || saving.value) return
  saving.value = true
  try {
    await channelsAPI.deleteModelPricingOverride(deleteTarget.value.model)
    deleteTarget.value = null
    appStore.showSuccess(t('admin.modelPricing.restoreSuccess'))
    await loadCatalog()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelPricing.restoreFailed')))
  } finally {
    saving.value = false
  }
}

function changePageSize(value: number) {
  pageSize.value = value
  page.value = 1
}

onMounted(loadCatalog)
</script>

<style scoped>
.pricing-ledger-grid {
  @apply grid gap-3 sm:grid-cols-2 xl:grid-cols-4;
}

.ledger-stat,
.ledger-file {
  @apply relative overflow-hidden rounded-xl border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800;
}

.ledger-stat {
  @apply grid grid-cols-[1fr_auto] items-end gap-x-3;
}

.ledger-stat::before,
.ledger-file::before {
  content: '';
  @apply absolute inset-y-0 left-0 w-1 bg-gray-300 dark:bg-dark-600;
}

.ledger-stat-accent::before { @apply bg-primary-500; }
.ledger-stat-warning::before { @apply bg-amber-500; }
.ledger-stat-warning strong { @apply text-amber-600 dark:text-amber-400; }

.ledger-stat strong {
  @apply row-span-2 font-mono text-2xl font-semibold tabular-nums text-gray-950 dark:text-white;
}

.ledger-stat > span:last-child,
.ledger-file > span:last-child {
  @apply mt-1 text-xs text-gray-400 dark:text-gray-500;
}

.ledger-label {
  @apply text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.ledger-file {
  @apply flex min-w-0 flex-col;
}

.ledger-file code {
  @apply mt-1 truncate text-sm font-semibold text-gray-800 dark:text-gray-200;
}

.rule-badge,
.deprecated-badge,
.unit-chip {
  @apply inline-flex rounded-md bg-violet-50 px-2 py-0.5 text-[11px] font-medium text-violet-700 dark:bg-violet-900/30 dark:text-violet-300;
}

.deprecated-badge {
  @apply inline-flex rounded-md bg-red-50 px-2 py-0.5 text-[11px] font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300;
}

.unit-chip {
  @apply whitespace-nowrap bg-gray-100 font-mono text-gray-600 dark:bg-dark-700 dark:text-gray-300;
}

.source-track {
  @apply relative flex min-w-[108px] items-center gap-2 text-xs font-medium;
}

.source-track::before {
  content: '';
  @apply absolute left-[5px] top-[-22px] h-5 w-px bg-gray-200 dark:bg-dark-600;
}

.source-dot { @apply h-2.5 w-2.5 rounded-full ring-4; }
.source-track-catalog { @apply text-gray-500 dark:text-gray-400; }
.source-track-catalog .source-dot { @apply bg-gray-400 ring-gray-100 dark:bg-dark-400 dark:ring-dark-700; }
.source-track-override { @apply text-primary-700 dark:text-primary-300; }
.source-track-override .source-dot { @apply bg-primary-500 ring-primary-100 dark:ring-primary-900/50; }
.source-track-missing { @apply text-amber-700 dark:text-amber-300; }
.source-track-missing .source-dot { @apply bg-amber-500 ring-amber-100 dark:ring-amber-900/50; }

.icon-action {
  @apply rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-primary-400;
}

.icon-action-danger {
  @apply hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400;
}
</style>
