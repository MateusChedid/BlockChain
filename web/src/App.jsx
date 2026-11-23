import React, { useEffect, useState } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Zap, ShieldCheck } from 'lucide-react';
import Layout from './components/Layout';
import { ToastProvider } from './components/Toast';
import { api, setSigningOrg } from './services/api';
import Dashboard from './pages/Dashboard';
import Governance from './pages/Governance';
import Meters from './pages/Meters';
import Reconciliation from './pages/Reconciliation';
import Billing from './pages/Billing';
import Sandbox from './pages/Sandbox';
import Households from './pages/Households';
import HouseholdApp from './pages/HouseholdApp';

const FALLBACK_ORGS = [
  { name: 'utility', displayName: 'Distribution Utility (DSO)', mspID: 'UtilityMSP' },
  { name: 'installer', displayName: 'Certified Solar Installer', mspID: 'InstallerMSP' },
  { name: 'consortium', displayName: 'WattLedger Consortium (Operator)', mspID: 'ConsortiumMSP' },
];

function App() {
  const [view, setView] = useState('admin');
  const [orgs, setOrgs] = useState(FALLBACK_ORGS);
  const [org, setOrg] = useState('utility');

  useEffect(() => {
    api.network().then((res) => res.data.orgs?.length && setOrgs(res.data.orgs)).catch(() => {});
  }, []);

  const changeOrg = (value) => {
    setOrg(value);
    setSigningOrg(value);
  };

  // The household app reads through the utility gateway.
  useEffect(() => {
    setSigningOrg(view === 'admin' ? org : 'utility');
  }, [view, org]);

  return (
    <ToastProvider>
      <BrowserRouter>
        <div className="flex min-h-screen flex-col">
          <div className="sticky top-0 z-40 flex flex-wrap items-center justify-between gap-4 border-b bg-white px-4 py-3 shadow-sm md:px-6">
            <div className="flex items-center gap-3">
              <div className="rounded-xl bg-slate-900 p-2 text-yellow-400">
                <Zap size={22} fill="currentColor" />
              </div>
              <div>
                <p className="text-lg font-bold leading-tight text-slate-900">WattLedger</p>
                <p className="text-xs text-slate-500">Auditable solar measurement on Hyperledger Fabric</p>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-3">
              {view === 'admin' && (
                <label className="flex items-center gap-2 rounded-full border border-slate-200 px-3 py-1.5 text-sm">
                  <ShieldCheck size={16} className="text-emerald-600" />
                  <span className="text-slate-500">Signing as</span>
                  <select className="bg-transparent font-semibold text-slate-900 focus:outline-none" value={org} onChange={(e) => changeOrg(e.target.value)}>
                    {orgs.map((o) => (
                      <option key={o.name} value={o.name}>
                        {o.mspID}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <div className="flex overflow-hidden rounded-full border border-slate-200">
                {[
                  ['admin', 'Consortium Console'],
                  ['household', 'Household App'],
                ].map(([id, label]) => (
                  <button
                    key={id}
                    type="button"
                    onClick={() => setView(id)}
                    className={`px-4 py-2 text-sm font-medium transition ${view === id ? 'bg-emerald-600 text-white' : 'bg-white text-slate-700 hover:bg-slate-100'}`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="flex-1">
            {view === 'admin' ? (
              <Layout>
                <Routes>
                  <Route path="/" element={<Dashboard />} />
                  <Route path="/meters" element={<Meters />} />
                  <Route path="/reconciliation" element={<Reconciliation />} />
                  <Route path="/billing" element={<Billing />} />
                  <Route path="/governance" element={<Governance />} />
                  <Route path="/sandbox" element={<Sandbox />} />
                  <Route path="/households" element={<Households />} />
                  <Route path="*" element={<Dashboard />} />
                </Routes>
              </Layout>
            ) : (
              <HouseholdApp />
            )}
          </div>
        </div>
      </BrowserRouter>
    </ToastProvider>
  );
}

export default App;
