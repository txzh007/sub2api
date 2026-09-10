<template>
  <div class="space-y-2" data-testid="image-bridge-field">
    <label class="input-label">{{ t('keys.imageBridgeModel') }}</label>
    <Select
      :model-value="modelValue"
      :options="options"
      :searchable="true"
      :disabled="loading"
      :placeholder="t('keys.imageBridgeDefault')"
      @update:model-value="value => emit('update:modelValue', value as string | null)"
    />
    <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('keys.imageBridgeHint') }}</p>
    <p v-if="loading" class="text-xs text-gray-500">{{ t('common.loading') }}</p>
    <div v-else-if="failed" class="flex items-center gap-2 text-xs text-red-600" role="alert">
      <span>{{ t('keys.imageBridgeLoadFailed') }}</span>
      <button type="button" class="underline" @click="loadModels">{{ t('keys.imageBridgeRetry') }}</button>
    </div>
    <p v-else-if="models.length === 0" class="text-xs text-amber-600 dark:text-amber-400">
      {{ t('keys.imageBridgeEmpty') }}
    </p>
    <p v-if="selectedUnavailable" class="text-xs text-amber-600 dark:text-amber-400" data-testid="image-bridge-unavailable-reason">
      {{ selectedUnavailable.model }}: {{ reasonLabel(selectedUnavailable.reason) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import Select, { type SelectOption } from '@/components/common/Select.vue'

const props = defineProps<{ modelValue: string | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: string | null] }>()
const { t } = useI18n()
const models = ref<string[]>([])
const defaultModel = ref('')
const availability = ref<Array<{ model: string; available: boolean; reason?: string }>>([])
const loading = ref(false)
const failed = ref(false)
const options = computed<SelectOption[]>(() => {
  const result: SelectOption[] = [
    { value: '', label: t('keys.imageBridgeDisabled') },
    { value: null, label: defaultModel.value ? `${t('keys.imageBridgeDefault')} · ${defaultModel.value}` : t('keys.imageBridgeDefault') },
    ...models.value.map(model => ({ value: model, label: model }))
  ]
  if (props.modelValue && !models.value.includes(props.modelValue)) {
    result.push({ value: props.modelValue, label: `${props.modelValue} · ${t('keys.imageBridgeUnavailable')}`, disabled: true })
  }
  return result
})
const selectedUnavailable = computed(() => {
  const selected = props.modelValue === null ? defaultModel.value : props.modelValue
  if (!selected) return null
  return availability.value.find(item => item.model === selected && !item.available) || null
})

function reasonLabel(reason?: string) {
  const keys: Record<string, string> = {
    no_provider: 'keys.imageBridgeReasonNoProvider',
    account_inactive: 'keys.imageBridgeReasonAccountInactive',
    account_unschedulable: 'keys.imageBridgeReasonAccountUnschedulable',
    wrong_account_purpose: 'keys.imageBridgeReasonWrongPurpose',
    not_in_group_allowlist: 'keys.imageBridgeReasonNotAllowed',
    temporarily_unavailable: 'keys.imageBridgeReasonTemporary'
  }
  return t(keys[reason || ''] || 'keys.imageBridgeUnavailable')
}

async function loadModels() {
  loading.value = true
  failed.value = false
  try {
    const result = await keysAPI.getImageBridgeModels()
    models.value = result.models
    defaultModel.value = result.default_model || ''
    availability.value = result.availability || []
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}
onMounted(loadModels)
</script>
