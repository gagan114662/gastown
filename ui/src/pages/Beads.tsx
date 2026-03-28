import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Badge } from '@/components/ui/badge'

interface Bead { id: string; title: string; status: string; rig?: string; assigned_to?: string }

export function Beads() {
  const { data: beads = [], isLoading } = useQuery({
    queryKey: ['beads'],
    queryFn: () => api.get<Bead[]>('/beads'),
    refetchInterval: 10000,
  })

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Beads</h1>
      {isLoading && <p className="text-slate-400">Loading…</p>}
      <div className="space-y-2">
        {beads.map((b) => (
          <div key={b.id} className="p-4 bg-white rounded border flex items-center justify-between">
            <div>
              <p className="font-medium">{b.title}</p>
              <p className="text-sm text-slate-500">{b.id} {b.rig ? `· ${b.rig}` : ''}</p>
            </div>
            <div className="flex gap-2 items-center">
              {b.assigned_to && <span className="text-xs text-slate-400">{b.assigned_to}</span>}
              <Badge variant="secondary">{b.status}</Badge>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
