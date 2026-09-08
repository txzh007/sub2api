import { describe, expect, it } from 'vitest'
import {
  displayToStoredPrice,
  resolvePricingCurrency,
  storedPriceToDisplay,
  usesUSDPricing
} from '../pricingCurrency'

describe('pricingCurrency', () => {
  it.each([
    ['openai', 'gpt-5', true],
    ['azure_ai', 'deployment-name', true],
    ['anthropic', 'claude-sonnet-4', true],
    ['xai', 'grok-4', true],
    ['vertex_ai', 'gemini-2.5-pro', true],
    ['', 'claude-*', true],
    ['', 'gemini-2.5-*', true],
    ['', 'grok-*', true],
    ['', 'gpt-5-*', true],
    ['dashscope', 'qwen3-max', false],
    ['deepseek', 'deepseek-chat', false],
    ['volcengine', 'doubao-seed-*', false]
  ])('classifies provider=%s model=%s', (provider, model, expected) => {
    expect(usesUSDPricing(provider, model)).toBe(expected)
  })

  it('uses RMB for all other model providers', () => {
    expect(resolvePricingCurrency('moonshot', 'kimi-k2')).toBe('CNY')
    expect(resolvePricingCurrency('zhipu', 'glm-4.5')).toBe('CNY')
  })

  it('keeps the same numeric price for RMB and USD without FX conversion', () => {
    const stored = displayToStoredPrice(2, 1_000_000)
    expect(stored).toBeCloseTo(0.000002)
    expect(storedPriceToDisplay(stored, 1_000_000)).toBeCloseTo(2)
  })
})
