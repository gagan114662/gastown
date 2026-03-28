import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface CostsData { total_usd?: number; today_usd?: number; by_rig?: Record<string, number>; by_role?: Record<string, number> }

export function Costs() {
  const { data: costs, isLoading } = useQuery({
    queryKey: ['costs-detail'],
    queryFn: () => api.get<CostsData>('/costs?by_rig=1&by_role=1'),
  })

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Costs</h1>
      {isLoading && <p className="text-slate-400">Loading…</p>}
      {costs && (
        <div className="space-y-6">
          <div className="grid grid-cols-2 gap-4">
            <Card>
              <CardHeader><CardTitle className="text-sm text-slate-500">Total</CardTitle></CardHeader>
              <CardContent><p className="text-3xl font-bold">${costs.total_usd?.toFixed(2) ?? '—'}</p></CardContent>
            </Card>
            <Card>
              <CardHeader><CardTitle className="text-sm text-slate-500">Today</CardTitle></CardHeader>
              <CardContent><p className="text-3xl font-bold">${costs.today_usd?.toFixed(2) ?? '—'}</p></CardContent>
            </Card>
          </div>
          {costs.by_rig && Object.keys(costs.by_rig).length > 0 && (
            <div>
              <h2 className="font-semibold mb-2">By Rig</h2>
              {Object.entries(costs.by_rig).map(([rig, cost]) => (
                <div key={rig} className="flex justify-between p-2 bg-white rounded border mb-1">
                  <span>{rig}</span><span>${cost.toFixed(2)}</span>
                </div>
              ))}
            </div>
          )}
          {costs.by_role && Object.keys(costs.by_role).length > 0 && (
            <div>
              <h2 className="font-semibold mb-2">By Role</h2>
              {Object.entries(costs.by_role).map(([role, cost]) => (
                <div key={role} className="flex justify-between p-2 bg-white rounded border mb-1">
                  <span>{role}</span><span>${cost.toFixed(2)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
