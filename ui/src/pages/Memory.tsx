import { useState } from 'react'
import { api } from '@/api/client'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

interface MemoryResult { content?: string; metadata?: Record<string, string>; distance?: number }
interface MemoryResults { transcripts: MemoryResult[]; beads: MemoryResult[]; docs: MemoryResult[] }

export function Memory() {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<MemoryResults | null>(null)
  const [loading, setLoading] = useState(false)

  const search = async () => {
    if (!query) return
    setLoading(true)
    const res = await api.get<MemoryResults>(`/memory/search?q=${encodeURIComponent(query)}`)
    setResults(res)
    setLoading(false)
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Memory Search</h1>
      <div className="flex gap-2 mb-6">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search across all agent memory…"
          onKeyDown={(e) => e.key === 'Enter' && search()}
          className="max-w-lg"
        />
        <Button onClick={search} disabled={loading}>{loading ? 'Searching…' : 'Search'}</Button>
      </div>

      {results && (
        <div className="space-y-6">
          {(['transcripts', 'beads', 'docs'] as const).map((type) =>
            (results[type]?.length ?? 0) > 0 ? (
              <div key={type}>
                <h2 className="text-lg font-semibold mb-3 capitalize">{type}</h2>
                <div className="space-y-2">
                  {results[type].map((r, i) => (
                    <Card key={i}>
                      <CardContent className="p-4">
                        <div className="flex gap-2 mb-1">
                          <Badge variant="secondary">{type.slice(0, -1)}</Badge>
                          {r.metadata?.rig && <Badge variant="outline">{r.metadata.rig}</Badge>}
                          {r.distance != null && (
                            <Badge variant="outline" className="text-slate-400">
                              {(1 - r.distance).toFixed(2)} match
                            </Badge>
                          )}
                        </div>
                        <p className="text-sm text-slate-700">{r.content?.slice(0, 200)}</p>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
            ) : null
          )}
        </div>
      )}
    </div>
  )
}
