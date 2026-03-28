const BASE = '/api/manage'

function getCsrfToken(): string {
  return document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content ?? ''
}

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      'X-Dashboard-Token': getCsrfToken(),
      ...opts?.headers,
    },
  })
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw new Error((err as { error: string }).error ?? resp.statusText)
  }
  return resp.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
}

export function streamTranscript(
  sessionId: string,
  workDir: string,
  onEvent: (e: Record<string, unknown>) => void
): () => void {
  const token = getCsrfToken()
  const url = `${BASE}/polecats/${sessionId}/stream?work_dir=${encodeURIComponent(workDir)}&token=${token}`
  const es = new EventSource(url)
  es.addEventListener('event', (e) => onEvent(JSON.parse(e.data) as Record<string, unknown>))
  es.addEventListener('error', () => es.close())
  return () => es.close()
}
