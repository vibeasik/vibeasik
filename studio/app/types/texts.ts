// Teks Website: teks yang tampil di website dikirim skrip browser ke engine, lalu bisa dijadikan todo "ubah teks".
import type { Todo } from './todo'

export interface TextSource {
  id: number
  project_id?: number
  component_id?: number
  name: string
  enabled: boolean
  last_ingest_at: string | null
  last_origin: string
  created_at: string
  updated_at: string
  project_name?: string
  component_name?: string
  texts: number
  pages: number
}

export interface TextSourceInput {
  name?: string
  project_id?: number | null
  component_id?: number | null
  enabled?: boolean
}

export interface TextEntry {
  id: number
  source_id: number
  hash: string
  page: string
  section: string
  selector: string
  tag: string
  text: string
  seen: number
  todo_id?: number
  todo_status?: string
  first_seen_at: string
  last_seen_at: string
}

export interface TextEntryPage {
  items: TextEntry[]
  total: number
  limit: number
  offset: number
}

export interface TextFacet {
  name: string
  count: number
}

export interface TextFacets {
  pages: TextFacet[]
  sections: TextFacet[]
}

export interface TextSDKInfo {
  ingest_url: string
  ingest_key: string
  script_path: string
  usages: { id: string, label: string, code: string }[]
  component_dir: string
  component_name: string
  curl_test: string
}

export interface TextEntryQuery {
  page?: string
  section?: string
  q?: string
  todo?: '' | 'has' | 'none'
  limit?: number
  offset?: number
}

export type TextTodoResult = { todo: Todo, already_exists: boolean }
