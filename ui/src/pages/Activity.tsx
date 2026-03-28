import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'

interface ActivityEntry { session_id: string; role?: string; rig?: string; event?: string; timestamp?: string }

export function Activity() {
  const { data: activity = [], isLoading } = useQuery({
    queryKey: ['activity'],
    queryFn: () => api.get<ActivityEntry[]>('/activity'),
    refetchInterval: 5000,
  })

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Activity</h1>
      {isLoading && <p className="text-slate-400">Loading…</p>}
      <div className="space-y-1">
        {activity.map((a, i) => (
          <div key={i} className="p-3 bg-white rounded border text-sm flex justify-between">
            <div>
              <span className="font-medium">{a.session_id}</span>
              {a.event && <span className="ml-2 text-slate-500">{a.event}</span>}
            </div>
            <div className="text-slate-400 text-xs">
              {a.rig && <span className="mr-2">{a.rig}</span>}
              {a.role && <span>{a.role}</span>}
            </div>
          </div>
        ))}
        {activity.length === 0 && !isLoading && <p className="text-slate-400">No recent activity.</p>}
      </div>
    </div>
  )
}
