import React, { useEffect, useState } from 'react';
import { ReceiptText } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Empty, SimulatedTag } from '../components/ui';
import { kwh, brl, rate, time, currentCycle } from '../services/format';

export const StatementView = ({ s }) => (
  <div className="rounded-2xl border border-slate-200 bg-white p-5">
    <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
      <div>
        <p className="text-sm font-semibold text-slate-900">{s.owner} · cycle {s.cycle}</p>
        <p className="text-xs text-slate-500">{s.distributor} · {s.statementID}</p>
      </div>
      <SimulatedTag />
    </div>
    <table className="min-w-full text-sm">
      <tbody>
        <tr className="border-b"><td className="py-1.5">Grid consumption (revenue meter)</td><td className="py-1.5 text-right">{kwh(s.consumedKWh)}</td></tr>
        <tr className="border-b"><td className="py-1.5">Injected this cycle (revenue meter)</td><td className="py-1.5 text-right">{kwh(s.injectedKWh)}</td></tr>
        <tr className="border-b"><td className="py-1.5">Credits used (available {kwh(s.creditsBeforeKWh)}, minimum charge kept)</td><td className="py-1.5 text-right">− {kwh(s.compensatedKWh)}</td></tr>
        <tr className="border-b"><td className="py-1.5">Billed energy × {rate(s.teBRL + s.tusdBRL)}</td><td className="py-1.5 text-right">{brl(s.energyChargeBRL)}</td></tr>
        <tr className="border-b"><td className="py-1.5">Fio B not compensated ({s.fioBPct}% × {rate(s.fioBBRL)} × credits used)</td><td className="py-1.5 text-right">{brl(s.fioBChargeBRL)}</td></tr>
        <tr className="font-semibold"><td className="py-2">Total before taxes</td><td className="py-2 text-right">{brl(s.totalBRL)}</td></tr>
        <tr className="text-slate-500"><td className="py-1">Same consumption without solar</td><td className="py-1 text-right">{brl(s.withoutSolarBRL)}</td></tr>
        <tr className="font-semibold text-emerald-700"><td className="py-1">Savings</td><td className="py-1 text-right">{brl(s.savingsBRL)}</td></tr>
        <tr className="text-slate-500"><td className="py-1">Credits carried to next cycles</td><td className="py-1 text-right">{kwh(s.creditsAfterKWh)}</td></tr>
      </tbody>
    </table>
    <p className="mt-2 text-xs text-slate-400">Taxes (ICMS, PIS/COFINS), tariff flags and public lighting are not included.</p>
  </div>
);

const Billing = () => {
  const toast = useToast();
  const [cycle, setCycle] = useState(currentCycle());
  const [recs, setRecs] = useState([]);
  const [statements, setStatements] = useState([]);
  const [tariffs, setTariffs] = useState(null);
  const [busy, setBusy] = useState('');

  const load = async () => {
    try {
      const [r, s, t] = await Promise.all([api.metering.reconciliations(), api.metering.statements(), api.governance.getTariffs()]);
      setRecs(r.data.reconciliations);
      setStatements(s.data.statements);
      setTariffs(t.data.tariffs);
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const owners = [...new Set(recs.filter((r) => r.cycle === cycle).map((r) => r.owner))];
  const billed = new Set(statements.filter((s) => s.cycle === cycle).map((s) => s.owner));

  const simulate = async (owner) => {
    setBusy(owner);
    try {
      const res = await api.metering.simulateBill({ owner, cycle });
      toast(`${owner}: simulated bill ${brl(res.data.statement.totalBRL)}, savings ${brl(res.data.statement.savingsBRL)}.`);
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    } finally {
      setBusy('');
    }
  };

  return (
    <>
      <Card
        title="Simulated compensation bill"
        subtitle="Net metering under Lei 14.300 with the 2026 Fio B rule, using only reconciled revenue-meter data. Nothing is sent to a real billing system."
        icon={<ReceiptText size={20} />}
        actions={<div className="flex items-end gap-2"><SimulatedTag /><div className="w-32"><label className="label">Cycle</label><input className="input" value={cycle} onChange={(e) => setCycle(e.target.value)} /></div></div>}
      >
        {tariffs && (
          <p className="mb-4 text-sm text-slate-600">
            Tariffs {tariffs.distributor}: TE {rate(tariffs.teBRL)} + TUSD {rate(tariffs.tusdBRL)} · Fio B {rate(tariffs.fioBBRL)} ({tariffs.fioBPct}% not compensated) · minimum {tariffs.availabilityKWh} kWh
          </p>
        )}
        {owners.length === 0 ? <Empty>No reconciled holder in {cycle}. Reconcile the meters first.</Empty> : (
          <div className="flex flex-wrap gap-2">
            {owners.map((o) => (
              <button key={o} className={billed.has(o) ? 'btn-ghost' : 'btn-dark'} disabled={billed.has(o) || busy === o} onClick={() => simulate(o)}>
                {billed.has(o) ? `${o} · bill generated` : `Simulate bill for ${o}`}
              </button>
            ))}
          </div>
        )}
      </Card>

      {statements.length === 0 ? <Card><Empty>No simulated bills yet.</Empty></Card> : (
        <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
          {statements.map((s) => <StatementView key={s.statementID} s={s} />)}
        </div>
      )}
      <p className="text-xs text-slate-400">Generated {statements[0] ? time(statements[0].generatedAt) : '—'} · UtilityMSP only</p>
    </>
  );
};

export default Billing;
