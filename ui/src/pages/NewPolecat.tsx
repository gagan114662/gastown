import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

interface Rig { name: string }
interface SimilarResult { content?: string; metadata?: Record<string, string> }
interface MemoryResults { transcripts: SimilarResult[]; beads: SimilarResult[]; docs: SimilarResult[] }
interface SpawnResult { session_id?: string }

export function NewPolecat() {
  const navigate = useNavigate()
  const [task, setTask] = useState('')
  const [rig, setRig] = useState('')
  const [similar, setSimilar] = useState<MemoryResults>({ transcripts: [], beads: [], docs: [] })

  const { data: rigs = [] } = useQuery({ queryKey: ['rigs'], queryFn: () => api.get<Rig[]>('/rigs') })

  useEffect(() => {
    if (task.length < 10) { setSimilar({ transcripts: [], beads: [], docs: [] }); return }
    const t = setTimeout(async () => {
      const res = await api.get<MemoryResults>(`/memory/search?q=${encodeURIComponent(task)}`)
      setSimilar(res)
    }, 500)
    return () => clearTimeout(t)
  }, [task])

  const spawnMutation = useMutation({
    mutationFn: () => api.post<SpawnResult>('/polecats', { task, rig }),
    onSuccess: (data) => navigate(`/manage/polecats/${data.session_id ?? ''}`),
  })

  const hasSimilar = similar.transcripts?.length > 0 || similar.beads?.length > 0

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-bold mb-6">Spawn Polecat</h1>

      <div className="space-y-4 mb-6">
        <div>
          <Label>Task description</Label>
          <Textarea
            value={task}
            onChange={(e) => setTask(e.target.value)}
            placeholder="Describe what the agent should do…"
            rows={4}
            className="mt-1"
          />
        </div>
        <div>
          <Label>Rig</Label>
          <Select onValueChange={setRig} value={rig}>
            <SelectTrigger className="mt-1"><SelectValue placeholder="Select rig…" /></SelectTrigger>
            <SelectContent>
              {rigs.map((r) => (
                <SelectItem key={r.name} value={r.name}>{r.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {hasSimilar && (
        <Card className="mb-6 border-yellow-300 bg-yellow-50">
          <CardHeader><CardTitle className="text-sm text-yellow-800">Similar past work found</CardTitle></CardHeader>
          <CardContent className="space-y-2">
            {similar.beads?.slice(0, 3).map((b, i) => (
              <div key={i} className="text-sm">
                <Badge variant="outline" className="mr-2">bead</Badge>
                {b.content?.split('\n')[0]?.slice(0, 80)}
              </div>
            ))}
            {similar.transcripts?.slice(0, 2).map((t, i) => (
              <div key={i} className="text-sm">
                <Badge variant="outline" className="mr-2">session</Badge>
                {t.content?.slice(0, 80)}
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Button
        onClick={() => spawnMutation.mutate()}
        disabled={!task || !rig || spawnMutation.isPending}
      >
        {spawnMutation.isPending ? 'Spawning…' : 'Spawn'}
      </Button>
    </div>
  )
}
