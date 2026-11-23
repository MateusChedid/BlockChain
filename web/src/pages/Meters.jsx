import React, { useEffect, useState } from 'react';
import { Cpu, Zap, ShieldAlert, Copy, Repeat, ShieldCheck, ShieldX, TrendingUp, Ban, AlertTriangle } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Badge, Empty, ScopeTag, meterTone } from '../components/ui';
import { kwh, num, time, shortId, currentCycle } from '../services/format';

const SCENARIOS = [
  { id: 'honest', label: 'Honest batch', help: 'Real value, valid signature', icon: <Zap size={16} />, cls: 'btn-primary', catches: 'Accepted, pending reconciliation' },
  { id: 'compromised', label: 'Compromised sensor (+40%)', help: 'Valid signature, inflated value', icon: <TrendingUp size={16} />, cls: 'btn-dark', catches: 'Passes the signature. Caught by peers and the revenue meter' },
  { id: 'gross', label: 'Implausible value', help: '8.5 kWh/kWp/day, valid signature', icon: <AlertTriangle size={16} />, cls: 'btn-dark', catches: 'Quarantined by the plausibility check' },
  { id: 'impossible', label: 'Impossible value', help: '14 kWh/kWp/day', icon: <Ban size={16} />, cls: 'btn-danger', catches: 'Rejected by the physical limit' },
  { id: 'tamper', label: 'Tampered in transit', help: 'Value x10 after signing', icon: <ShieldAlert size={16} />, cls: 'btn-danger', catches: 'Rejected by the signature' },
  { id: 'forged', label: 'Cloned device', help: 'Uncertified key', icon: <Copy size={16} />, cls: 'btn-danger', catches: 'Rejected by the signature' },
  { id: 'replay', label: 'Replay last batch', help: 'Same period twice', icon: <Repeat size={16} />, cls: 'btn-danger', catches: 'Rejected, period already reported' },
];

const Meters = () => {
  const toast = useToast();
  const [meters, setMeters] = useState([]);
  const [states, setStates] = useState({});
  const [users, setUsers] = useState([]);
  const [readings, setReadings] = useState([]);
  const [form, setForm] = useState({ meterID: '', owner: '', consumerUnit: '', substation: 'SUB-01', capacityKWp: '4' });
  const [selected, setSelected] = useState('');
  const [sim, setSim] = useState(null);
  const [log, setLog] = useState([]);
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const [m, u, r, s] = await Promise.all([api.meters.list(), api.users.list(), api.metering.readings(), api.metering.states()]);
      setMeters(m.data.meters);
      setUsers(u.data.users);
      setReadings(r.data.readings);
      setStates(Object.fromEntries(s.data.states.map((x) => [x.meterID, x])));
      if (!selected && m.data.meters.length) setSelected(m.data.meters[0].meterID);
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (selected) api.meters.simulator(selected).then((r) => setSim(r.data.simulator)).catch(() => setSim(null));
  }, [selected, readings]);

  const register = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.meters.register({ ...form, capacityKWp: parseFloat(form.capacityKWp) });
      toast(`Meter ${form.meterID} certified. Public key and capacity are on-chain.`);
      setSelected(form.meterID);
      setForm({ ...form, meterID: '' });
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  const send = async (scenario) => {
    if (!selected) return;
    setBusy(true);
    try {
      const res = await api.meters.submitReading(selected, { scenario });
      const r = res.data.reading;
      setLog((l) => [{ ...res.data, scenario, meterID: selected, at: new Date() }, ...l].slice(0, 10));
      toast(r.status === 'FLAGGED' ? `Recorded but QUARANTINED: ${r.reason}` : `Batch accepted: ${num(r.kwh, 1)} kWh pending reconciliation.`, r.status === 'FLAGGED' ? 'error' : 'success');
      load();
    } catch (err) {
      const data = err?.response?.data || {};
      setLog((l) => [{ ...data, accepted: false, scenario, meterID: selected, error: errorMessage(err), at: new Date() }, ...l].slice(0, 10));
      toast(`Rejected by chaincode: ${errorMessage(err)}`, 'error');
    } finally {
      setBusy(false);
    }
  };

  const prosumers = users.filter((u) => u.type === 'prosumer');

  return (
    <>
      <div className="grid gap-6 xl:grid-cols-3">
        <Card title="Certify an IoT meter" subtitle="UtilityMSP or InstallerMSP · key pair created in the meter HSM" icon={<Cpu size={20} />}>
          <form onSubmit={register} className="space-y-3">
            <div><label className="label">Meter ID</label><input className="input" placeholder="M-ALICE-02" value={form.meterID} onChange={(e) => setForm({ ...form, meterID: e.target.value })} required /></div>
            <div>
              <label className="label">Holder (prosumer)</label>
              <select className="input" value={form.owner} required onChange={(e) => {
                const u = users.find((x) => x.id === e.target.value);
                setForm({ ...form, owner: e.target.value, substation: u?.substation || form.substation, consumerUnit: u?.consumerUnit || form.consumerUnit });
              }}>
                <option value="">Select a household…</option>
                {prosumers.map((u) => <option key={u.id} value={u.id}>{u.displayName} ({u.id})</option>)}
              </select>
            </div>
            <div className="grid grid-cols-3 gap-2">
              <div><label className="label">UC</label><input className="input" value={form.consumerUnit} onChange={(e) => setForm({ ...form, consumerUnit: e.target.value })} required /></div>
              <div><label className="label">Substation</label><input className="input" value={form.substation} onChange={(e) => setForm({ ...form, substation: e.target.value })} required /></div>
              <div><label className="label">kWp</label><input className="input" type="number" step="0.1" value={form.capacityKWp} onChange={(e) => setForm({ ...form, capacityKWp: e.target.value })} required /></div>
            </div>
            <button className="btn-dark w-full" disabled={busy}>Register meter on-chain</button>
          </form>
        </Card>

        <Card title="IoT meter simulator" subtitle="Sends the next weekly batch. A valid signature authenticates the source, not the physical truth." icon={<Zap size={20} />} className="xl:col-span-2" actions={<ScopeTag />}>
          <div className="grid gap-3 md:grid-cols-3">
            <div>
              <label className="label">Meter</label>
              <select className="input" value={selected} onChange={(e) => setSelected(e.target.value)}>
                {meters.map((m) => <option key={m.meterID} value={m.meterID}>{m.meterID} · {m.owner} · {m.capacityKWp} kWp</option>)}
              </select>
            </div>
            <div className="text-sm"><p className="label">Next period</p><p className="font-semibold">{sim?.nextPeriod || '—'}</p></div>
            <div className="text-sm"><p className="label">Physical truth this cycle</p><p className="font-semibold">{sim ? kwh(sim.physicalTruthKWh?.[currentCycle()] || 0) : '—'}</p><p className="text-xs text-slate-500">known only to the simulator</p></div>
          </div>
          <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            {SCENARIOS.map((s) => (
              <button key={s.id} className={`${s.cls} flex-col !items-start !gap-1 py-3 text-left`} disabled={busy || !selected} onClick={() => send(s.id)} title={s.catches}>
                <span className="flex items-center gap-2">{s.icon}{s.label}</span>
                <span className="text-[11px] font-normal opacity-80">{s.help}</span>
              </button>
            ))}
          </div>
          <div className="mt-5 space-y-2">
            {log.length === 0 && <Empty>Send a batch to see the chaincode decision.</Empty>}
            {log.map((e, i) => {
              const flagged = e.accepted && e.reading?.status === 'FLAGGED';
              const tone = !e.accepted ? 'border-rose-200 bg-rose-50' : flagged ? 'border-amber-200 bg-amber-50' : 'border-emerald-200 bg-emerald-50';
              return (
                <div key={i} className={`rounded-xl border p-3 text-sm ${tone}`}>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="flex items-center gap-2 font-semibold">
                      {e.accepted ? (flagged ? <AlertTriangle size={16} className="text-amber-600" /> : <ShieldCheck size={16} className="text-emerald-600" />) : <ShieldX size={16} className="text-rose-600" />}
                      {e.accepted ? (flagged ? 'QUARANTINED' : 'ACCEPTED') : 'REJECTED'} · {e.meterID} · {SCENARIOS.find((s) => s.id === e.scenario)?.label}
                    </span>
                    <span className="text-xs text-slate-500">{e.at.toLocaleTimeString()}</span>
                  </div>
                  <p className="mt-1 text-slate-700">
                    {e.accepted ? (flagged ? e.reading.reason : `${num(e.reading.kwh, 1)} kWh for ${e.reading.periodStart} to ${e.reading.periodEnd} (${num(e.reading.yieldKWhKWpDay)} kWh/kWp/day) · tx ${shortId(e.reading.txID)}`) : e.error}
                  </p>
                  {e.batch && (
                    <p className="mt-1 break-all font-mono text-[11px] text-slate-500">
                      signed "{e.batch.signed.message}" · submitted {e.batch.submittedKWh} kWh · physical {e.batch.realKWh || '—'} kWh
                    </p>
                  )}
                </div>
              );
            })}
          </div>
        </Card>
      </div>

      <Card title="Certified meters" subtitle="Registry in governance-cc, health in energy-cc">
        {meters.length === 0 ? <Empty>No meters registered.</Empty> : (
          <table className="min-w-full">
            <thead><tr className="border-b"><th className="th">Meter</th><th className="th">Holder</th><th className="th">UC</th><th className="th">Substation</th><th className="th">Capacity</th><th className="th">Reported up to</th><th className="th">Health</th></tr></thead>
            <tbody>
              {meters.map((m) => {
                const st = states[m.meterID];
                return (
                  <tr key={m.meterID} className="border-b last:border-0">
                    <td className="td font-mono text-xs">{m.meterID}</td>
                    <td className="td">{m.owner}</td>
                    <td className="td">{m.consumerUnit}</td>
                    <td className="td"><Badge>{m.substation}</Badge></td>
                    <td className="td">{m.capacityKWp} kWp</td>
                    <td className="td">{st?.lastPeriodEnd || '—'}</td>
                    <td className="td">{!m.active ? <Badge tone="rose">REVOKED</Badge> : <Badge tone={meterTone(st?.status || 'OK')}>{st?.status || 'OK'}</Badge>}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </Card>

      <Card title="Signed batches" subtitle="Each period can be reported only once">
        {readings.length === 0 ? <Empty>No readings yet.</Empty> : (
          <div className="overflow-x-auto">
            <table className="min-w-full">
              <thead><tr className="border-b"><th className="th">Meter</th><th className="th">Period</th><th className="th">Energy</th><th className="th">Yield</th><th className="th">Status</th><th className="th">Reconciled</th><th className="th">Transaction</th><th className="th">Recorded</th></tr></thead>
              <tbody>
                {readings.slice(0, 14).map((r) => (
                  <tr key={r.meterID + r.readingID} className="border-b last:border-0">
                    <td className="td font-mono text-xs">{r.meterID}</td>
                    <td className="td text-xs">{r.periodStart} → {r.periodEnd}</td>
                    <td className="td">{kwh(r.kwh)}</td>
                    <td className="td">{num(r.yieldKWhKWpDay)} kWh/kWp/d</td>
                    <td className="td"><Badge tone={r.status === 'FLAGGED' ? 'amber' : 'emerald'}>{r.status}</Badge></td>
                    <td className="td">{r.reconciled ? 'yes' : 'pending'}</td>
                    <td className="td font-mono text-xs">{shortId(r.txID)}</td>
                    <td className="td text-xs text-slate-500">{time(r.recordedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
};

export default Meters;
