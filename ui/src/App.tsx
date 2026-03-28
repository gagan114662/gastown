import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Polecats } from './pages/Polecats'
import { PolecatDetail } from './pages/PolecatDetail'
import { NewPolecat } from './pages/NewPolecat'
import { Beads } from './pages/Beads'
import { Rigs } from './pages/Rigs'
import { Costs } from './pages/Costs'
import { Activity } from './pages/Activity'
import { Memory } from './pages/Memory'

const qc = new QueryClient()

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          <Route path="/manage" element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="polecats" element={<Polecats />} />
            <Route path="polecats/new" element={<NewPolecat />} />
            <Route path="polecats/:id" element={<PolecatDetail />} />
            <Route path="beads" element={<Beads />} />
            <Route path="rigs" element={<Rigs />} />
            <Route path="costs" element={<Costs />} />
            <Route path="activity" element={<Activity />} />
            <Route path="memory" element={<Memory />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
