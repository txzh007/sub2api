import { apiClient } from '../client'

export interface PelicanKeyOption {
  id: number
  name: string
  group_id: number
  group_name: string
}

export interface PelicanRun {
  id: number
  admin_user_id: number
  api_key_id: number
  group_id: number
  group_name: string
  model: string
  prompt: string
  output_mode: 'svg' | 'text'
  batch_id: string
  prompt_version: string
  request_id: string
  usage_request_id: string | null
  status: string
  response_text?: string
  svg?: string
  error_message?: string
  duration_ms: number
  input_tokens: number | null
  output_tokens: number | null
  total_cost: number | null
  actual_cost: number | null
  created_at: string
  finished_at: string | null
}

const base = '/admin/pelican-tests'

export async function options() {
  const { data } = await apiClient.get<{ keys: PelicanKeyOption[]; prompt: string }>(`${base}/options`)
  return data
}

export async function models(keyId: number) {
  const { data } = await apiClient.get<{ models: string[] }>(`${base}/models`, { params: { key_id: keyId } })
  return data.models
}

export async function createRun(keyId: number, model: string) {
  const { data } = await apiClient.post<{ id: number; request_id: string }>(`${base}/runs`, { key_id: keyId, model })
  return data
}

export async function listRuns(groupId?: number, model?: string, beforeId?: number) {
  const { data } = await apiClient.get<{ items: PelicanRun[]; next_before_id: number }>(`${base}/runs`, {
    params: { group_id: groupId || undefined, model: model || undefined, before_id: beforeId || undefined }
  })
  return data
}

export async function getRun(id: number) {
  const { data } = await apiClient.get<PelicanRun>(`${base}/runs/${id}`)
  return data
}

export interface PelicanTemplate {
  id: number
  name: string
  prompt: string
  output_mode: 'svg' | 'text'
  is_default: boolean
}

export interface PelicanBatchInput {
  key_id: number
  models: string[]
  prompt: string
  output_mode: 'svg' | 'text'
}

export async function createBatch(input: PelicanBatchInput) {
  const { data } = await apiClient.post<{ batch_id: string; ids: number[] }>(`${base}/batches`, input)
  return data
}
export async function getBatch(id: string) {
  const { data } = await apiClient.get<{ items: PelicanRun[] }>(`${base}/batches/${id}`)
  return data.items
}
export async function cancelBatch(id: string) { await apiClient.post(`${base}/batches/${id}/cancel`) }
export async function cancelRun(id: number) { await apiClient.post(`${base}/runs/${id}/cancel`) }
export async function templates() {
  const { data } = await apiClient.get<{ items: PelicanTemplate[] }>(`${base}/templates`)
  return data.items
}
export async function saveTemplate(input: Omit<PelicanTemplate, 'id'>, id?: number) {
  const { data } = id
    ? await apiClient.put<PelicanTemplate>(`${base}/templates/${id}`, input)
    : await apiClient.post<PelicanTemplate>(`${base}/templates`, input)
  return data
}
export async function deleteTemplate(id: number) { await apiClient.delete(`${base}/templates/${id}`) }

export default { options, models, createRun, listRuns, getRun, createBatch, getBatch, cancelBatch, cancelRun, templates, saveTemplate, deleteTemplate }
