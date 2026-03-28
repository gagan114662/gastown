import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, streamTranscript } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

interface Polecat {
  id: string
  rig: string
  task?: string
  work_dir?: string
  status: string
}

interface TranscriptEvent {
  type: string
  role: string
  content?: string
  input_tokens?: number
  output_tokens?: number
}

export function PolecatDetail() {
  const { id } = useParams<{ id: string }>()
  const qc = useQueryClient()
  const [events, setEvents] = useState<TranscriptEvent[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)

  const { data: polecat } = useQuery({
    queryKey: ['polecats', id],
    queryFn: () => api.get<Polecat[]>('/polecats').then((ps) => ps.find((p) => p.id === id)),
    refetchInterval: 3000,
  })

  useEffect(() => {
    if (!id || !polecat?.work_dir) return
    const stop = streamTranscript(id, polecat.work_dir, (e) => {
      setEvents((prev) => [...prev, e as unknown as TranscriptEvent])
      setTimeout(() => bottomRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
    })
    return stop
  }, [id, polecat?.work_dir])

  const stopMutation = useMutation({
    mutationFn: () => api.post(`/polecats/${id}/stop`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['polecats'] }),
  })

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">{id}</h1>
          <p className="text-slate-500">{polecat?.rig}</p>
        </div>
        <div className="flex gap-2 items-center">
          <Badge variant={polecat?.status === 'running' ? 'default' : 'secondary'}>{polecat?.status}</Badge>
          {polecat?.status === 'running' && (
            <Button variant="destructive" onClick={() => stopMutation.mutate()}>Stop</Button>
          )}
        </div>
      </div>

      {polecat?.task && (
        <div className="bg-white rounded border p-4 mb-4">
          <p className="text-sm font-medium text-slate-500 mb-1">Task</p>
          <p>{polecat.task}</p>
        </div>
      )}

      <div className="bg-slate-900 rounded p-4 h-[60vh] overflow-y-auto font-mono text-sm">
        {events.length === 0 && <p className="text-slate-500">Waiting for events…</p>}
        {events.map((e, i) => (
          <div key={i} className={`mb-2 ${e.role === 'assistant' ? 'text-green-400' : 'text-slate-400'}`}>
            {e.type === 'usage' ? (
              <span className="text-slate-600 text-xs">
                [{e.input_tokens}in / {e.output_tokens}out tokens]
              </span>
            ) : (
              <span>{e.content}</span>
            )}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
