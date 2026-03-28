import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function Dashboard() {
  const { data: rigs } = useQuery({ queryKey: ['rigs'], queryFn: () => api.get<unknown[]>('/rigs') })
  const { data: costs } = useQuery({ queryKey: ['costs'], queryFn: () => api.get<{ today_usd?: number }>('/costs') })
  const { data: polecats } = useQuery({ queryKey: ['polecats'], queryFn: () => api.get<Array<{ status: string }>>('/polecats') })

  const activeCount = polecats?.filter((p) => p.status === 'running').length ?? 0

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Dashboard</h1>
      <div className="grid grid-cols-3 gap-4 mb-8">
        <Card>
          <CardHeader><CardTitle className="text-sm text-slate-500">Active Polecats</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{activeCount}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-slate-500">Rigs</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{Array.isArray(rigs) ? rigs.length : '—'}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-slate-500">Today's Spend</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">${costs?.today_usd?.toFixed(2) ?? '—'}</p></CardContent>
        </Card>
      </div>
    </div>
  )
}
