import React, { useEffect, useState } from 'react';
import { Sun, ShieldCheck, AlertTriangle, ReceiptText, Cpu, Server, RefreshCw, CheckCircle2, Clock3 } from 'lucide-react';
import { api } from '../services/api';
import { Card, Stat, Badge, Empty, reconTone } from '../components/ui';
import { kwh, brl, pct, time } from '../services/format';

const DEMONSTRATED = [
  'Signed IoT readings from certified meters (ECDSA P-256)',
  'Physical-limit and plausibility checks, replay protection',
  'Reconciliation against the utility revenue meter (legal source of truth)',
  'Peer audit of yield per kWp on the same substation',
  'Simulated compensation bill (Lei 14.300, Fio B 60% in 2026)',
  'Immutable audit trail shared by utility, installers and operator',
];

const FUTURE = [
  ['P2P sale or transfer of credits between holders', 'Not allowed today (Lei 14.300 art. 28, REN 1.059). Needs an ANEEL regulatory sandbox'],
  ['Integration with the real billing system', 'Needs a utility agreement and certification'],
  ['Meter data as legal evidence', 'Needs INMETRO-certified hardware and a metering agreement'],
  ['Retail energy offers to households', 'Depends on the free market opening (Lei 15.269/2025, residential by 2028)'],
];

const Dashboard = () => {
  const [d, setD] = useState({ meters: [], readings: [], recs: [], statements: [], network: null });
  const [loading, setLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    try {
      const [m, r, rc, st, n] = await Promise.all([
        api.meters.list(), api.metering.readings(), api.metering.reconciliations(), api.metering.statements(), api.network(),
      ]);
      setD({ meters: m.data.meters, readings: r.data.readings, recs: rc.data.reconciliations, statements: st.data.statements, network: n.data });
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, []);

  const iot = d.readings.reduce((s, r) => s + r.kwh, 0);
  const official = d.recs.reduce((s, r) => s + r.utilityInjectedKWh, 0);
  const flagged = d.readings.filter((r) => r.status === 'FLAGGED').length;
  const discrepancies = d.recs.filter((r) => r.status === 'DISCREPANCY').length;
  const savings = d.statements.reduce((s, x) => s + x.savingsBRL, 0);

  return (
    <>
      <div className="flex justify-end">
        <button className="btn-ghost" onClick={load} disabled={loading}>
          <RefreshCw size={16} className={loading ? 'animate-spin' : ''} /> Refresh from ledger
        </button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <Stat label="Certified meters" value={d.meters.filter((m) => m.active).length} hint={`${d.readings.length} signed batches recorded`} icon={<Cpu size={20} />} tone="slate" />
        <Stat label="Energy reported by IoT" value={kwh(iot)} hint="Signed by the meters, not yet legal truth" icon={<Sun size={20} />} tone="amber" />
        <Stat label="Official credits reconciled" value={kwh(official)} hint="From the utility revenue meter" icon={<ShieldCheck size={20} />} />
        <Stat label="Anomalies detected" value={flagged + discrepancies} hint={`${flagged} implausible batches · ${discrepancies} revenue-meter mismatches`} icon={<AlertTriangle size={20} />} tone="rose" />
        <Stat label="Simulated bills" value={d.statements.length} hint={`Savings vs. no solar ${brl(savings)}`} icon={<ReceiptText size={20} />} tone="violet" />
        <Stat label="Reconciled cycles" value={d.recs.length} hint="Each one an auditable ledger record" icon={<CheckCircle2 size={20} />} tone="sky" />
      </div>

      <div className="grid gap-6 xl:grid-cols-2">
        <Card title="Demonstrated in this MVP" subtitle="Runs end to end on the Fabric network" icon={<CheckCircle2 size={20} />}>
          <ul className="space-y-2 text-sm">
            {DEMONSTRATED.map((x) => (
              <li key={x} className="flex items-start gap-2"><CheckCircle2 size={16} className="mt-0.5 shrink-0 text-emerald-600" />{x}</li>
            ))}
          </ul>
        </Card>
        <Card title="Future capabilities" subtitle="Legally or contractually dependent, not claimed as working" icon={<Clock3 size={20} />}>
          <ul className="space-y-3 text-sm">
            {FUTURE.map(([what, why]) => (
              <li key={what}>
                <p className="font-semibold text-slate-800">{what}</p>
                <p className="text-slate-500">{why}</p>
              </li>
            ))}
          </ul>
        </Card>
      </div>

      <div className="grid gap-6 xl:grid-cols-3">
        <Card title="Latest reconciliations" subtitle="IoT readings vs. utility revenue meter" className="xl:col-span-2">
          {d.recs.length === 0 ? <Empty>No cycle reconciled yet.</Empty> : (
            <table className="min-w-full">
              <thead><tr className="border-b"><th className="th">Meter</th><th className="th">Cycle</th><th className="th">IoT</th><th className="th">Revenue meter</th><th className="th">Difference</th><th className="th">Status</th><th className="th">When</th></tr></thead>
              <tbody>
                {d.recs.slice(0, 6).map((r) => (
                  <tr key={r.meterID + r.cycle} className="border-b last:border-0">
                    <td className="td font-mono text-xs">{r.meterID}</td>
                    <td className="td">{r.cycle}</td>
                    <td className="td">{kwh(r.iotKWh)}</td>
                    <td className="td">{kwh(r.utilityInjectedKWh)}</td>
                    <td className="td">{pct(r.diffPct)}</td>
                    <td className="td"><Badge tone={reconTone(r.status)}>{r.status}</Badge></td>
                    <td className="td text-xs text-slate-500">{time(r.reconciledAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
        <Card title="Consortium network" subtitle={`Channel ${d.network?.channel || 'energychannel'}`} icon={<Server size={20} />}>
          <div className="space-y-3">
            {(d.network?.orgs || []).map((o) => (
              <div key={o.name} className="rounded-xl border border-slate-100 bg-slate-50 p-3">
                <div className="flex items-center justify-between">
                  <p className="text-sm font-semibold text-slate-800">{o.displayName}</p>
                  <Badge tone="emerald">{o.mspID}</Badge>
                </div>
                <p className="mt-1 font-mono text-xs text-slate-500">{o.peerEndpoint}</p>
              </div>
            ))}
            <p className="text-xs text-slate-500">Endorsement policy MAJORITY (2 of 3 organizations). Ordering by Raft in the MVP.</p>
          </div>
        </Card>
      </div>
    </>
  );
};

export default Dashboard;
