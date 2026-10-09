<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import '../batch-test.css'
defineProps<{ hours: 1 | 24; autoRefresh: boolean; loading: boolean; error: boolean; updatedAt: string; monitoring: boolean }>()
defineEmits<{ 'update:hours': [hours: 1 | 24]; 'update:autoRefresh': [enabled: boolean]; refresh: [] }>()
const { t, locale } = useI18n()
</script>
<template>
  <div class="tt-batch aa-toolbar">
    <span class="aa-label"><Icon name="chartBar" size="sm" />{{ t('admin.accountActivity.title') }}</span>
    <div class="bt-segments"><button :aria-pressed="hours === 1" @click="$emit('update:hours', 1)">{{ t('admin.accountActivity.hour') }}</button><button :aria-pressed="hours === 24" @click="$emit('update:hours', 24)">{{ t('admin.accountActivity.day') }}</button></div>
    <span class="aa-hint" :class="{ 'aa-error': error }">{{ error ? t('admin.accountActivity.stale') : !monitoring ? t('admin.accountActivity.monitoringOff') : t('admin.accountActivity.hint') }}</span>
    <span v-if="updatedAt" class="aa-time">{{ t('admin.accountActivity.updated', { time: new Date(updatedAt).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' }) }) }}</span>
    <label><input type="checkbox" :checked="autoRefresh" @change="$emit('update:autoRefresh', ($event.target as HTMLInputElement).checked)" />{{ t('admin.accountActivity.auto') }}</label>
    <button class="bt-button" :aria-label="t('admin.accountActivity.refresh')" :title="t('admin.accountActivity.refresh')" :disabled="loading" @click="$emit('refresh')"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" /></button>
  </div>
</template>
<style scoped>
.aa-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding-top: 14px; font-size: 11px; }
.aa-label { display: flex; align-items: center; gap: 7px; font-weight: 600; }
.aa-label svg { color: var(--bt-accent); }
.aa-hint { color: var(--bt-muted); flex: 1; font-size: 10px; }
.aa-error { color: #c76850; }
.aa-time { color: var(--bt-muted); font-size: 10px; }
label { display: flex; gap: 5px; align-items: center; color: var(--bt-muted); cursor: pointer; }
.aa-toolbar .bt-button { min-height: 30px; padding: 5px 7px; }
.aa-toolbar .bt-segments button { padding: 5px 9px; font-size: 11px; }
@media (max-width: 1100px) { .aa-hint,.aa-time { display: none; } }
</style>
