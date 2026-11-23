import React, { useEffect, useState } from 'react';
import { Sun, ShieldCheck, Hourglass, AlertTriangle, History, Lock, Cpu } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Badge, Empty, ScopeTag, meterTone, reconTone } from '../components/ui';
import { kwh, num, pct, time, shortId } from '../services/format';
import { StatementView } from './Billing';

const HouseholdApp = () => {
  const toast = useToast();
  const [users, setUsers] = useState([]);
  const [userId, setUserId] = useState('');
  const [d, setD] = useState(null);

  useEffect(() => {
    api.users.list()
      .then((u) => {
        setUsers(u.data.users);
        if (u.data.users.length) setUserId(u.data.users[0].id);
      })
      .catch((e) => toast(errorMessage(e), 'error'));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!userId) return;
    Promise.all([
      api.metering.account(userId), api.metering.history(userId), api.meters.list(), api.metering.states(),
      api.metering.readings(), api.metering.reconciliations(), api.metering.statements(),
    ])
      .then(([a, h, m, s, r, rc, st]) => {
        const meters = m.data.meters.filter((x) => x.owner === userId);
        const ids = new Set(meters.map((x) => x.meterID));
        setD({
          account: a.data.account,
          history: h.data.history,
          meters,
          states: Object.fromEntries(s.data.states.map((x) => [x.meterID, x])),
          readings: r.data.readings.filter((x) => ids.has(x.meterID)),
          recs: rc.data.reconciliations.filter((x) => x.owner === userId),
          statement: st.data.statements.find((x) => x.owner === userId),
        });
      })
      .catch((e) => toast(errorMessage(e), 'error'));
  }, [userId]); // eslint-disable-line react-hooks/exhaustive-deps

  const user = users.find((u) => u.id === userId);
  const inspection = d?.meters.some((m) => d.states[m.meterID]?.status === 'INSPECTION_REQUIRED');

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-sm font-semibold uppercase tracking-wide text-emerald-600">Household App</p>
          <h1 className="text-2xl font-bold text-slate-900 md:text-3xl">Every kWh you generate, verifiable</h1>
          <p className="text-slate-500">Read directly from the Hyperledger Fabric ledger shared by your utility and installer.</p>
        </div>
        <div className="w-72">
          <label className="label">Signed in as</label>
          <select className="input" value={userId} onChange={(e) => setUserId(e.target.value)}>
            {users.map((u) => <option key={u.id} value={u.id}>{u.displayName} · {u.type} · {u.consumerUnit || u.substation}</option>)}
          </select>
        </div>
      </header>

      {users.length === 0 && <Card><Empty>No households yet. Run scripts/seed-demo.sh.</Empty></Card>}

      {d && user && (
        <>
          {inspection && (
            <div className="flex items-start gap-3 rounded-2xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
              <AlertTriangle size={20} className="mt-0.5 shrink-0" />
              <p>Your meter reported more energy than the utility revenue meter measured. A technician visit is required. Your credits are based only on the revenue meter and new IoT readings stay in quarantine until the inspection.</p>
            </div>
          )}

          <section className="grid gap-4 md:grid-cols-3">
            <div className="rounded-2xl bg-gradient-to-br from-emerald-600 via-emerald-500 to-teal-500 p-6 text-white shadow-lg">
              <div className="flex items-start justify-between">
                <p className="text-sm uppercase tracking-widest text-emerald-100">Official credits</p>
                <ShieldCheck size={24} />
              </div>
              <p className="mt-2 text-4xl font-bold">{num(d.account.creditKWh, 1)} <span className="text-xl font-semibold">kWh</span></p>
              <p className="mt-2 text-sm text-emerald-50">Confirmed by the utility revenue meter, available for your next bills</p>
            </div>
            <div className="card">
              <div className="flex items-start justify-between"><p className="text-sm font-semibold">Waiting for reconciliation</p><Hourglass size={20} className="text-amber-600" /></div>
              <p className="mt-2 text-3xl font-bold text-slate-900">{kwh(d.account.pendingKWh)}</p>
              <p className="mt-1 text-xs text-slate-500">Signed by your meter this cycle</p>
            </div>
            <div className="card">
              <div className="flex items-start justify-between"><p className="text-sm font-semibold">In quarantine</p><AlertTriangle size={20} className="text-rose-600" /></div>
              <p className="mt-2 text-3xl font-bold text-slate-900">{kwh(d.account.quarantinedKWh)}</p>
              <p className="mt-1 text-xs text-slate-500">Implausible readings, never become credits on their own</p>
            </div>
          </section>

          <div className="grid gap-6 lg:grid-cols-2">
            <div className="space-y-4">
              <Card title="Latest bill" subtitle="Lei 14.300 compensation with your utility's tariffs" icon={<Sun size={20} />}>
                {d.statement ? <StatementView s={d.statement} /> : <Empty>No bill simulated yet for this household.</Empty>}
              </Card>
              <Card title="Trade credits with neighbors" subtitle="Not available today" icon={<Lock size={20} />} actions={<ScopeTag future />}>
                <p className="text-sm text-slate-600">
                  Brazilian rules only let your credits offset your own consumption. Peer-to-peer trading is planned for an ANEEL regulatory sandbox and for the free-market opening for households (Lei 15.269/2025).
                </p>
              </Card>
            </div>

            <div className="space-y-4">
              <Card title="My meters and cycles" icon={<Cpu size={20} />}>
                {d.meters.length === 0 ? <Empty>No solar meter (consumer only).</Empty> : (
                  <div className="space-y-3">
                    {d.meters.map((m) => (
                      <div key={m.meterID} className="flex items-center justify-between rounded-xl border border-slate-100 p-3 text-sm">
                        <div><p className="font-semibold">{m.meterID}</p><p className="text-xs text-slate-500">{m.capacityKWp} kWp · {m.consumerUnit} · {m.substation}</p></div>
                        <Badge tone={meterTone(d.states[m.meterID]?.status || 'OK')}>{d.states[m.meterID]?.status || 'OK'}</Badge>
                      </div>
                    ))}
                    {d.recs.map((r) => (
                      <div key={r.meterID + r.cycle} className="flex items-center justify-between rounded-xl bg-slate-50 p-3 text-sm">
                        <div><p className="font-semibold">Cycle {r.cycle}</p><p className="text-xs text-slate-500">Meter {kwh(r.iotKWh)} · utility {kwh(r.utilityInjectedKWh)} ({pct(r.diffPct)})</p></div>
                        <Badge tone={reconTone(r.status)}>{r.status}</Badge>
                      </div>
                    ))}
                  </div>
                )}
              </Card>
              <Card title="Ledger history" subtitle="Immutable history of your account (GetHistoryForKey)" icon={<History size={20} />}>
                {d.history.length === 0 ? <Empty>No ledger entries yet.</Empty> : (
                  <ol className="space-y-3 border-l-2 border-emerald-100 pl-4">
                    {d.history.slice(0, 8).map((h) => (
                      <li key={h.txID} className="relative">
                        <span className="absolute -left-[23px] top-1.5 h-3 w-3 rounded-full bg-emerald-500" />
                        <p className="text-sm text-slate-800">{h.action}</p>
                        <p className="text-xs text-slate-500">{time(h.timestamp)} · tx {shortId(h.txID)}</p>
                      </li>
                    ))}
                  </ol>
                )}
              </Card>
            </div>
          </div>
        </>
      )}
    </div>
  );
};

export default HouseholdApp;
