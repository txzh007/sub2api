<template>
  <div v-if="eligible" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
    <label class="flex items-center gap-3 text-sm font-medium text-gray-900 dark:text-gray-100">
      <input
        type="checkbox"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        :checked="enabled"
        :disabled="!enabled && !canEnable"
        @change="setEnabled(($event.target as HTMLInputElement).checked)"
      />
      {{ t('imageProviders.oauthToggle') }}
    </label>
    <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('imageProviders.oauthToggleHint') }}</p>
    <p v-if="!canEnable && !enabled" class="mt-2 text-xs text-amber-600">{{ t('imageProviders.oauthGroupUnavailable') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Group } from '@/types'

const props = defineProps<{
  modelValue: number[]
  groups: Group[]
  platform?: string
  type?: string
  shadow?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: number[]] }>()
const { t } = useI18n()
const eligible = computed(() => props.type === 'oauth' && !props.shadow && ['openai', 'gemini', 'grok'].includes(props.platform || ''))
const imageGroup = computed(() => props.groups.find(group => group.system_role === 'image_generation'))
const enabled = computed(() => !!imageGroup.value && props.modelValue.includes(imageGroup.value.id))
const compatibleGroup = computed(() => !!imageGroup.value && (imageGroup.value.platform === 'composite' || imageGroup.value.platform === props.platform))
const canEnable = computed(() => eligible.value && compatibleGroup.value && imageGroup.value?.status === 'active' && imageGroup.value.allow_image_generation)

function setEnabled(value: boolean) {
  if (!imageGroup.value || (value && !canEnable.value)) return
  const ids = props.modelValue.filter(id => id !== imageGroup.value!.id)
  if (value) ids.push(imageGroup.value.id)
  emit('update:modelValue', ids)
}

// Creating an account can change platform/type after opt-in. Do not carry the
// hidden image group selection to an unsupported credential type or platform.
watch(() => [props.platform, props.type, props.shadow], () => {
  if (enabled.value && (!eligible.value || !compatibleGroup.value)) setEnabled(false)
})
</script>
