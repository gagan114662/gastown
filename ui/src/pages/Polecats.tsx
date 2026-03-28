import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

interface Polecat {
  id: string
  rig: string
  task?: string
  status: string
}

export function Polecats() {
  const { data: polecats = [], isLoading } = useQuery({
    queryKey: ['polecats'],
    queryFn: () => api.get<Polecat[]>('/polecats'),
    refetchInterval: 5000,
  })

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Polecats</h1>
        <Link to="/manage/polecats/new"><Button>+ Spawn</Button></Link>
      </div>
      {isLoading && <p className="text-slate-400">Loading…</p>}
      <div className="space-y-2">
        {polecats.map((p) => (
          <Link key={p.id} to={`/manage/polecats/${p.id}`}
            className="block p-4 bg-white rounded border hover:shadow transition-shadow">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium">{p.id}</p>
                <p className="text-sm text-slate-500">{p.rig} · {p.task?.slice(0, 80)}</p>
              </div>
              <Badge variant={p.status === 'running' ? 'default' : 'secondary'}>{p.status}</Badge>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
