import { Sidebar } from './Sidebar'
import { Outlet } from 'react-router-dom'

export function Layout() {
  return (
    <div className="flex">
      <Sidebar />
      <main className="ml-56 flex-1 p-6 min-h-screen bg-slate-50">
        <Outlet />
      </main>
    </div>
  )
}
