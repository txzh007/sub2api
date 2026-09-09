import { describe, expect, it } from 'vitest'
import type { ModelPricingCatalogEntry } from '@/api/admin/channels'
import {
  filterPricingEntriesByScope,
  matchesModelPattern,
  materializeGroupPricingEntries,
  resolveEffectiveGroupModels,
  type PricingGroupSource
} from '@/utils/modelPricingGroupScope'

function catalogEntry(model: string, input = 0, output = 0, wildcard = false): ModelPricingCatalogEntry {
  return {
    model,
    litellm_provider: 'test-provider',
    mode: 'chat',
    deprecated: false,
    input_cost_per_token: input,
    input_cost_per_token_priority: 0,
    output_cost_per_token: output,
    output_cost_per_token_priority: 0,
    cache_creation_input_token_cost: 0,
    cache_creation_input_token_cost_priority: 0,
    cache_creation_input_token_cost_above_1hr: 0,
    cache_read_input_token_cost: 0,
    cache_read_input_token_cost_priority: 0,
    output_cost_per_image: 0,
    output_cost_per_video: 0,
    output_cost_per_image_token: 0,
    input_cost_per_image_token: 0,
    long_context_input_token_threshold: 0,
    long_context_input_cost_multiplier: 0,
    long_context_output_cost_multiplier: 0,
    supports_prompt_caching: false,
    supports_service_tier: false,
    token_pricing_absent: input === 0 && output === 0,
    overridden: wildcard,
    wildcard
  }
}

const group: PricingGroupSource = {
  id: 7,
  name: '国内模型',
  platform: 'dashscope',
  model_allowlist: { enabled: true, models: ['qwen-*', 'deepseek-chat'] },
  model_pricing: []
}

describe('model pricing group scope', () => {
  it('matches exact and trailing wildcard model rules case-insensitively', () => {
    expect(matchesModelPattern('Qwen-Plus', 'qwen-*')).toBe(true)
    expect(matchesModelPattern('deepseek-chat', 'DEEPSEEK-CHAT')).toBe(true)
    expect(matchesModelPattern('deepseek-reasoner', 'deepseek-chat')).toBe(false)
  })

  it('uses candidate models constrained by the group allowlist', () => {
    expect(resolveEffectiveGroupModels(group, ['qwen-plus', 'deepseek-chat', 'glm-4'])).toEqual([
      'deepseek-chat',
      'qwen-plus'
    ])
  })

  it('keeps exact manually priced models that are absent from the remote catalog', () => {
    const configured = {
      ...group,
      model_allowlist: { enabled: false, models: [] },
      model_pricing: [{ models: ['doubao-pro'] }]
    }
    expect(resolveEffectiveGroupModels(configured, ['qwen-plus'])).toEqual(['doubao-pro', 'qwen-plus'])
  })

  it('materializes missing group models and applies the longest wildcard price rule', () => {
    const entries = materializeGroupPricingEntries(
      [catalogEntry('qwen-*', 0.000001, 0.000002, true), catalogEntry('deepseek-chat', 0.000003, 0.000004)],
      [{ group, candidates: ['qwen-plus', 'deepseek-chat', 'glm-4'] }]
    )

    expect(entries.map(item => item.model)).toEqual(['deepseek-chat', 'qwen-plus'])
    expect(entries[0].synthetic).toBeUndefined()
    expect(entries[1]).toMatchObject({
      model: 'qwen-plus',
      inherited_from: 'qwen-*',
      input_cost_per_token: 0.000001,
      token_pricing_absent: false,
      group_ids: [7]
    })
  })

  it('adds a zero-priced editable row for a group model missing from the catalog', () => {
    const openGroup = { ...group, model_allowlist: { enabled: false, models: [] } }
    const entries = materializeGroupPricingEntries([], [{ group: openGroup, candidates: ['glm-4'] }])
    expect(entries[0]).toMatchObject({
      model: 'glm-4',
      litellm_provider: 'dashscope',
      token_pricing_absent: true,
      synthetic: true
    })
  })

  it('keeps a model used by an active group even when the catalog marks it deprecated', () => {
    const deprecated = { ...catalogEntry('deepseek-chat', 1, 1), deprecated: true }
    const entries = materializeGroupPricingEntries([deprecated], [{ group, candidates: ['deepseek-chat'] }])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ model: 'deepseek-chat', deprecated: true, group_ids: [7] })
  })

  it('filters by one group and identifies catalog rules unused by active groups', () => {
    const secondGroup = { ...group, id: 8, name: '另一个分组', model_allowlist: { enabled: false, models: [] } }
    const catalog = [catalogEntry('qwen-*', 1, 1, true), catalogEntry('unused-model', 1, 1)]
    const entries = materializeGroupPricingEntries(catalog, [
      { group, candidates: ['qwen-plus'] },
      { group: secondGroup, candidates: ['deepseek-chat'] }
    ])

    expect(filterPricingEntriesByScope(catalog, entries, 'group:8').map(item => item.model)).toEqual(['deepseek-chat'])
    expect(filterPricingEntriesByScope(catalog, entries, 'unused').map(item => item.model)).toEqual(['unused-model'])
  })
})
