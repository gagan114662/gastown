import { NavLink } from 'react-router-dom'
import { LayoutDashboard, Bot, Boxes, Server, DollarSign, Activity, Search } from 'lucide-react'

const links = [
  { to: '/manage', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/manage/polecats', label: 'Polecats', icon: Bot, end: false },
  { to: '/manage/beads', label: 'Beads', icon: Boxes, end: false },
  { to: '/manage/rigs', label: 'Rigs', icon: Server, end: false },
  { to: '/manage/costs', label: 'Costs', icon: DollarSign, end: false },
  { to: '/manage/activity', label: 'Activity', icon: Activity, end: false },
  { to: '/manage/memory', label: 'Memory', icon: Search, end: false },
]

export function Sidebar() {
  return (
    <nav className="w-56 h-screen bg-slate-900 text-slate-100 flex flex-col p-4 gap-1 fixed left-0 top-0">
      <div className="text-lg font-bold mb-6 px-2">⛽ Gastown</div>
      {links.map(({ to, label, icon: Icon, end }) => (
        <NavLink
          key={to}
          to={to}
          end={end}
          className={({ isActive }) =>
            `flex items-center gap-2 px-3 py-2 rounded text-sm transition-colors ${
              isActive ? 'bg-slate-700 text-white' : 'text-slate-400 hover:bg-slate-800 hover:text-white'
            }`
          }
        >
          <Icon size={16} /> {label}
        </NavLink>
      ))}
    </nav>
  )
}
