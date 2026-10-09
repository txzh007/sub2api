<template>
  <BaseDialog :show="show && !singleMode" :title="t('admin.batchTest.batch')" width="wide" @close="emit('close')">
    <div class="tt-batch bt-account-identity">
      <div class="bt-row bt-spacer">
        <span class="bt-account-avatar" aria-hidden="true">{{ account?.name?.slice(0, 1).toUpperCase() }}</span>
        <div class="bt-spacer"><strong class="bt-account-name">{{ account?.name }}</strong><div class="bt-row"><span class="bt-muted bt-small">{{ account?.platform }}</span><span class="bt-badge">{{ account?.type === 'apikey' ? 'API Key' : account?.type === 'oauth' ? 'OAuth' : account?.type }}</span></div></div>
      </div>
      <button class="bt-button" @click="singleMode = true">{{ t('admin.batchTest.single') }}<Icon name="arrowRight" size="xs" /></button>
    </div>
    <p v-if="error" class="mb-3 text-sm text-red-500">{{ error }} <button class="underline" @click="loadModels">{{ t('admin.batchTest.retry') }}</button></p>
    <AccountModelBatchTest v-if="show && !singleMode && account" :account="account" :models="models" :loading="loading" :actions-target="actionsTarget" />
    <template #footer><div ref="actionsTarget" class="w-full" /></template>
  </BaseDialog>
  <SingleAccountTestModal :show="show && singleMode" :account="account" @close="emit('close')" />
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SingleAccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountModelBatchTest from './AccountModelBatchTest.vue'
import { adminAPI } from '@/api/admin'
import type { Account, ClaudeModel } from '@/types'
const props = defineProps<{ show: boolean; account: Account | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t } = useI18n()
const actionsTarget = ref<HTMLElement | null>(null)
const singleMode = ref(false), models = ref<ClaudeModel[]>([]), loading = ref(false), error = ref('')
let generation = 0
async function loadModels() {
  const token = ++generation, id = props.account?.id
  models.value = []; error.value = ''
  if (!props.show || !id) { loading.value = false; return }
  loading.value = true
  try { const result = await adminAPI.accounts.getAvailableModels(id); if (token === generation) models.value = result }
  catch (e) { if (token === generation) error.value = e && typeof e === 'object' && 'message' in e ? String(e.message) : t('common.unknownError') }
  finally { if (token === generation) loading.value = false }
}
watch(() => [props.show, props.account?.id], () => { singleMode.value = false; void loadModels() }, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>
<style scoped>
.bt-account-identity { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 14px; padding-bottom: 20px; margin-bottom: 22px; border-bottom: 1px solid var(--bt-line); }
.bt-account-avatar { display: grid; place-items: center; width: 44px; height: 44px; flex-shrink: 0; background: var(--bt-tint); border: 1px solid var(--bt-selected); border-radius: 11px; color: var(--bt-accent); font-size: 20px; font-weight: 650; }
.bt-account-name { display: block; margin-bottom: 4px; font-size: 15px; font-weight: 650; overflow-wrap: anywhere; }
</style>
