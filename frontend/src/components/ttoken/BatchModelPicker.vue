<template>
  <div class="bt-model-picker">
    <div class="bt-search">
      <Icon name="search" size="sm" aria-hidden="true" />
      <input v-model="search" class="bt-input" :placeholder="t('admin.batchTest.search')" :aria-label="t('admin.batchTest.search')" />
    </div>
    <div class="bt-row bt-between bt-wrap bt-small">
      <div class="bt-row">
        <button class="bt-link" :disabled="disabled || loading || !filtered.length" @click="selectVisible">{{ t('admin.batchTest.selectVisible') }}</button>
        <button class="bt-link bt-muted" :disabled="disabled || !selected.length" @click="emit('update:selected', [])">{{ t('admin.batchTest.clear') }}</button>
      </div>
      <span class="bt-muted">{{ t('admin.batchTest.selected', { count: selected.length }) }}<span v-if="limit"> / {{ limit }}</span></span>
    </div>
    <div class="bt-model-list">
      <label v-for="model in filtered" :key="model.id" class="bt-model" :class="{ 'is-selected': selected.includes(model.id) }">
        <input type="checkbox" :checked="selected.includes(model.id)" :aria-label="model.label || model.id" :disabled="disabled || (!!limit && selected.length >= limit && !selected.includes(model.id))" @change="toggle(model.id)" />
        <span class="bt-spacer">
          <span class="bt-model-name" :title="model.label || model.id">{{ model.label || model.id }}</span>
          <span v-if="model.label && model.label !== model.id" class="bt-model-id" :title="model.id">{{ model.id }}</span>
        </span>
        <span class="bt-model-mark" aria-hidden="true"><Icon :name="selected.includes(model.id) ? 'check' : 'terminal'" size="sm" /></span>
      </label>
      <div v-if="!filtered.length" class="bt-notice"><Icon name="search" size="sm" /><span>{{ loading ? t('common.loading') : t('admin.batchTest.noModels') }}</span></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  models: { id: string; label?: string }[]
  selected: string[]
  loading?: boolean
  disabled?: boolean
  limit?: number
}>()
const emit = defineEmits<{ (e: 'update:selected', value: string[]): void }>()
const { t } = useI18n()
const search = ref('')
const filtered = computed(() => props.models.filter(model => `${model.id} ${model.label || ''}`.toLowerCase().includes(search.value.trim().toLowerCase())))
function selectVisible() {
  const ids = [...new Set([...props.selected, ...filtered.value.map(model => model.id)])]
  emit('update:selected', props.limit ? ids.slice(0, props.limit) : ids)
}
function toggle(id: string) {
  const exists = props.selected.includes(id)
  if (props.disabled || (!exists && props.limit && props.selected.length >= props.limit)) return
  emit('update:selected', exists ? props.selected.filter(value => value !== id) : [...props.selected, id])
}
</script>
