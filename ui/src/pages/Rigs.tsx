import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Badge } from '@/components/ui/badge'

interface Rig { name: string; status?: string; workers?: number }

export function Rigs() {
  const { data: rigs = [], isLoading } = useQuery({
    queryKey: ['rigs'],
    queryFn: () => api.get<Rig[]>('/rigs'),
  })

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Rigs</h1>
      {isLoading && <p className="text-slate-400">Loading…</p>}
      <div className="space-y-2">
        {rigs.map((r) => (
          <div key={r.name} className="p-4 bg-white rounded border flex items-center justify-between">
            <p className="font-medium">{r.name}</p>
            <div className="flex gap-2 items-center">
              {r.workers != null && <span className="text-sm text-slate-500">{r.workers} workers</span>}
              {r.status && <Badge variant="secondary">{r.status}</Badge>}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
