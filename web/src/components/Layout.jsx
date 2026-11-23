import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import { LayoutDashboard, Cpu, Scale, ReceiptText, Settings2, Users, FlaskConical } from 'lucide-react';

export const navItems = [
  { name: 'Network Dashboard', path: '/', icon: <LayoutDashboard size={18} /> },
  { name: 'Meters & Readings', path: '/meters', icon: <Cpu size={18} /> },
  { name: 'Reconciliation & Audit', path: '/reconciliation', icon: <Scale size={18} /> },
  { name: 'Simulated Billing', path: '/billing', icon: <ReceiptText size={18} /> },
  { name: 'Governance & Tariffs', path: '/governance', icon: <Settings2 size={18} /> },
  { name: 'Households', path: '/households', icon: <Users size={18} /> },
  { name: 'P2P Sandbox (future)', path: '/sandbox', icon: <FlaskConical size={18} />, future: true },
];

const Layout = ({ children }) => {
  const location = useLocation();
  const current = navItems.find((i) => i.path === location.pathname);

  return (
    <div className="flex min-h-[calc(100vh-72px)] flex-col md:flex-row">
      <aside className="w-full bg-slate-900 text-white md:w-64">
        <div className="border-b border-slate-700 p-5">
          <p className="text-sm font-bold tracking-wide">Consortium Console</p>
          <p className="mt-1 text-xs text-slate-400">Utility · Installers · Operator</p>
        </div>
        <nav className="flex gap-1 overflow-x-auto p-3 md:block">
          {navItems.map((item) => (
            <Link
              key={item.path}
              to={item.path}
              className={`mb-1 flex items-center gap-3 whitespace-nowrap rounded-lg px-3 py-2.5 text-sm transition ${
                location.pathname === item.path ? 'bg-emerald-600 text-white' : item.future ? 'text-violet-300 hover:bg-slate-800' : 'text-slate-300 hover:bg-slate-800'
              }`}
            >
              {item.icon}
              <span>{item.name}</span>
            </Link>
          ))}
        </nav>
      </aside>
      <div className="flex flex-1 flex-col">
        <header className="border-b bg-white px-6 py-4">
          <h2 className="text-xl font-semibold text-slate-900">{current?.name || 'Portal'}</h2>
        </header>
        <main className="flex-1 space-y-6 overflow-auto p-4 md:p-6">{children}</main>
      </div>
    </div>
  );
};

export default Layout;
