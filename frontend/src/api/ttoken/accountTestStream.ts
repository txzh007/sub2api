import { buildApiUrl } from '@/api/client'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'

export interface AccountTestEvent {
  type: string
  model?: string
  text?: string
  success?: boolean
  error?: string
  image_url?: string
  audio_url?: string
  video_url?: string
}

/** One account/model request. The caller owns cancellation and timeout. */
export async function streamAccountTest(
  accountID: number,
  body: { model_id: string; prompt: string; mode?: string },
  signal: AbortSignal,
  onEvent: (event: AccountTestEvent) => void
) {
  const response = await fetch(buildApiUrl(`/admin/accounts/${accountID}/test`), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
      'Content-Type': 'application/json',
      [ADMIN_UI_REQUEST_HEADER]: '1'
    },
    body: JSON.stringify(body), signal
  })
  if (!response.ok) {
    const detail = await response.json().catch(() => null)
    throw new Error(detail?.message || detail?.error?.message || `HTTP ${response.status}`)
  }
  const reader = response.body?.getReader()
  if (!reader) throw new Error('Empty response stream')
  const decoder = new TextDecoder()
  let buffer = ''
  let terminal = false
  const parse = (line: string) => {
    if (!line.startsWith('data:')) return
    const data = line.slice(5).trim()
    if (!data || data === '[DONE]') return
    const event = JSON.parse(data) as AccountTestEvent
    onEvent(event)
    terminal = event.type === 'test_complete' || event.type === 'error'
  }
  try {
    while (!terminal) {
      const { done, value } = await reader.read()
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) { parse(line.trimEnd()); if (terminal) break }
      if (done) { if (buffer && !terminal) parse(buffer.trimEnd()); break }
      if (buffer.length > 8 * 1024 * 1024) throw new Error('Response event is too large')
    }
    if (!terminal) throw new Error('Stream ended before the test completed')
  } finally {
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
