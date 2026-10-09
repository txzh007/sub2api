<template>
  <article class="bt-result-card" :class="{ 'is-selected': selected }">
    <header class="bt-card-head">
      <div class="bt-spacer">
        <h3>{{ run.model }}</h3>
        <p class="bt-muted">{{ run.group_name }} · {{ new Date(run.created_at).toLocaleString() }}</p>
      </div>
      <span class="bt-status" :data-status="run.status">{{ t(`admin.pelicanTest.status.${run.status}`) }}</span>
    </header>
    <div class="bt-card-preview" :class="{ 'has-svg': previewURL }">
      <img v-if="previewURL" :src="previewURL" :alt="`${run.model} SVG`" />
      <pre v-else-if="run.output_mode === 'text' && run.response_text" class="bt-mono">{{ run.response_text }}</pre>
      <div v-else class="bt-card-placeholder">
        <Icon :name="active ? 'clock' : 'exclamationCircle'" size="lg" />
        <p>{{ active ? t(`admin.pelicanTest.status.${run.status}`) : run.error_message || t('admin.pelicanTest.noPreview') }}</p>
      </div>
    </div>
    <dl class="bt-card-metrics">
      <div><dt>{{ t('admin.pelicanTest.duration') }}</dt><dd class="bt-mono">{{ run.duration_ms ? `${(run.duration_ms / 1000).toFixed(1)}s` : '—' }}</dd></div>
      <div><dt>{{ t('admin.pelicanTest.tokens') }}</dt><dd class="bt-mono">{{ run.input_tokens == null && run.output_tokens == null ? '—' : `${run.input_tokens ?? 0} / ${run.output_tokens ?? 0}` }}</dd></div>
      <div :title="t('admin.pelicanTest.costHint')"><dt>{{ t('admin.pelicanTest.cost') }}</dt><dd class="bt-mono">{{ (run.actual_cost ?? run.total_cost)?.toFixed(6) ?? '—' }}</dd></div>
    </dl>
    <div class="bt-card-actions bt-row bt-between bt-wrap">
      <label class="bt-row bt-small bt-muted"><input type="checkbox" :checked="selected" :aria-label="`${t('admin.pelicanTest.compare')} ${run.model}`" @change="emit('compare')" />{{ t('admin.batchTest.design.addCompare') }}</label>
      <div class="bt-row">
        <button class="bt-link" @click="emit('restore')">{{ t('admin.pelicanTest.restore') }}</button>
        <button v-if="active && owned" class="bt-link bt-danger" @click="emit('cancel')">{{ t('admin.pelicanTest.cancelOne') }}</button>
        <button v-else-if="!active && owned" class="bt-link bt-row" :disabled="busy" @click="emit('retry')"><Icon name="refresh" size="xs" />{{ t('admin.batchTest.retry') }}</button>
      </div>
    </div>
    <details class="bt-card-details">
      <summary class="bt-muted bt-small">{{ t('admin.pelicanTest.details') }}<Icon name="chevronDown" size="xs" /></summary>
      <div class="bt-card-raw">
        <span class="bt-badge">{{ t('admin.pelicanTest.promptSnapshot') }} · {{ run.output_mode || 'svg' }}</span>
        <pre class="bt-mono">{{ run.prompt }}</pre>
        <p class="bt-muted bt-mono">{{ t('admin.pelicanTest.requestId') }}: {{ run.usage_request_id || run.request_id }}</p>
        <pre class="bt-mono">{{ run.response_text || run.error_message }}</pre>
      </div>
    </details>
  </article>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores/auth'
import { sanitizeSvg } from '@/utils/sanitize'
import type { PelicanRun } from '@/api/admin/pelicanTests'
const props = defineProps<{ run: PelicanRun; selected: boolean; busy: boolean }>()
const emit = defineEmits<{ (e: 'compare' | 'restore' | 'cancel' | 'retry'): void }>()
const { t } = useI18n(), auth = useAuthStore(), previewURL = ref('')
const active = computed(() => ['running', 'queued'].includes(props.run.status))
const owned = computed(() => props.run.admin_user_id === auth.user?.id)
watch(() => props.run.svg, value => {
  if (previewURL.value) URL.revokeObjectURL(previewURL.value)
  const svg = sanitizeSvg(value || '')
  previewURL.value = svg ? URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' })) : ''
}, { immediate: true })
onBeforeUnmount(() => { if (previewURL.value) URL.revokeObjectURL(previewURL.value) })
</script>

<style scoped>
.bt-result-card { min-width: 0; overflow: hidden; border: 1px solid var(--bt-line); border-radius: 11px; background: var(--bt-bg); }
.bt-result-card.is-selected { border-color: var(--bt-selected); box-shadow: 0 0 0 1px var(--bt-selected); }
.bt-card-head { display: flex; align-items: start; gap: 12px; padding: 16px; }
.bt-card-head h3 { font-size: 13px; font-weight: 650; overflow-wrap: anywhere; }
.bt-card-head p { margin-top: 5px; font-size: 10px; }
.bt-card-preview { display: flex; align-items: center; justify-content: center; height: 280px; padding: 20px; background: var(--bt-soft); border-block: 1px solid var(--bt-line); overflow: auto; }
.bt-card-preview.has-svg { background: #fff; }
.bt-card-preview img { width: 100%; height: 100%; object-fit: contain; }
.bt-card-preview > pre { width: 100%; height: 100%; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; line-height: 1.9; }
.bt-card-placeholder { display: flex; align-items: center; flex-direction: column; gap: 14px; max-width: 300px; text-align: center; color: var(--bt-muted); }
.bt-card-placeholder p { font-size: 12px; line-height: 1.8; overflow-wrap: anywhere; }
.bt-card-metrics { display: grid; grid-template-columns: 1fr 1.3fr 1fr; gap: 12px; padding: 14px 16px; }
.bt-card-metrics dt { font-size: 10px; color: var(--bt-muted); }
.bt-card-metrics dd { margin-top: 4px; font-size: 12px; overflow-wrap: anywhere; }
.bt-card-actions { padding: 13px 16px; border-top: 1px solid var(--bt-line); }
.bt-card-details { border-top: 1px solid var(--bt-line); }
.bt-card-details summary { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; cursor: pointer; list-style: none; }
.bt-card-details summary::-webkit-details-marker { display: none; }
.bt-card-details[open] summary > svg { transform: rotate(180deg); }
.bt-card-raw { padding: 0 16px 16px; }
.bt-card-raw pre { margin-block: 12px; max-height: 220px; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; padding: 12px; border-radius: 8px; background: var(--bt-soft); font-size: 11px; line-height: 1.8; }
.bt-card-raw > p { font-size: 10px; overflow-wrap: anywhere; }
</style>
