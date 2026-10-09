<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { activityLatency, activityRate, type AccountActivity } from '@/api/ttoken/accountActivity'
defineProps<{ stats?: AccountActivity; loading: boolean; error: boolean; monitoring: boolean }>()
defineEmits<{ open: [] }>()
const { t } = useI18n()
</script>

<template>
  <button class="aa-cell" :aria-label="t('admin.accountActivity.open')" @click="$emit('open')">
    <span v-if="loading && !stats" class="aa-muted">{{ t('common.loading') }}</span>
    <span v-else-if="error" class="aa-warning">{{ t('admin.accountActivity.unavailable') }}</span>
    <template v-else-if="stats">
      <span class="aa-cell-top"><strong :class="{ 'aa-warning': stats.failed > 0 || stats.recovered > 0 }">{{ activityRate(stats, monitoring) }}</strong><span class="aa-muted">{{ stats.success + stats.failed }} {{ t('admin.accountActivity.requestsUnit') }}</span></span>
      <span v-if="stats.failed || stats.recovered" class="aa-warning">{{ t('admin.accountActivity.shortErrors', { failed: stats.failed, recovered: stats.recovered }) }}</span>
      <span v-else class="aa-muted">{{ t('admin.accountActivity.ttft') }} {{ activityLatency(stats.avg_first_token_ms) }}</span>
    </template>
    <span v-else class="aa-muted">—</span>
  </button>
</template>

<style scoped>
.aa-cell { min-width: 122px; display: flex; flex-direction: column; gap: 5px; text-align: left; font-size: 11px; border-radius: 6px; padding: 5px 7px; margin-left: -7px; }
.aa-cell:hover { background: #287d7310; }
.aa-cell:focus-visible { outline: 2px solid #287d73; outline-offset: 2px; }
.aa-cell-top { display: flex; gap: 12px; align-items: baseline; }
.aa-cell strong { font: 600 14px ui-monospace, monospace; color: #287d73; }
.aa-muted { color: #788494; }
.aa-cell .aa-warning { color: #bc6b30; }
:global(.dark) .aa-cell strong { color: #8cd8c8; }
:global(.dark) .aa-cell .aa-warning { color: #eab17b; }
</style>
