export type PricingCurrency = 'USD' | 'CNY'

const usdProviderMarkers = [
  'openai',
  'azure',
  'anthropic',
  'claude',
  'xai',
  'grok',
  'gemini',
  'google',
  'vertex'
]

const usdModelPatterns = [
  /^(?:[^/]+\/)*(?:claude|gemini|grok)(?:[-_.:/]|\d|\*)/,
  /^(?:[^/]+\/)*(?:gpt|chatgpt|dall-e)(?:[-_.:/]|\d|\*)/,
  /^(?:[^/]+\/)*o[134](?:[-_.:/]|\d|\*)/,
  /^(?:[^/]+\/)*text-embedding-(?:ada|3)(?:[-_.:/]|\d|\*)/
]

export function usesUSDPricing(provider: string, model = ''): boolean {
  const normalizedProvider = provider.trim().toLowerCase()
  if (usdProviderMarkers.some(marker => normalizedProvider.includes(marker))) return true

  const normalizedModel = model.trim().toLowerCase()
  return usdModelPatterns.some(pattern => pattern.test(normalizedModel))
}

export function resolvePricingCurrency(provider: string, model = ''): PricingCurrency {
  return usesUSDPricing(provider, model) ? 'USD' : 'CNY'
}

export function pricingCurrencySymbol(currency: PricingCurrency): '$' | '¥' {
  return currency === 'USD' ? '$' : '¥'
}

export function storedPriceToDisplay(value: number, scale: number): number {
  return value * scale
}

export function displayToStoredPrice(value: number, scale: number): number {
  return value / scale
}
