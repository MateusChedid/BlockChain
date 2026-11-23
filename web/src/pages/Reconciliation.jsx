import React, { useEffect, useState } from 'react';
import { Scale, Gauge, RefreshCw } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Badge, Empty, ScopeTag, reconTone, meterTone, flagLabel } from '../components/ui';
import { kwh, num, pct, time, currentCycle } from '../services/format';

const Reconciliation = () => {
  const toast = useToast();
  const [cycle, setCycle] = useState(currentCycle());
  const [rows, setRows] = useState([]);
  const [recs, setRecs] = useState([]);
  const [busy, setBusy] = useState('');

  const load = async (c = cycle) => {
    try {
      const [a, r] = await Promise.all([api.metering.audit(c), api.metering.reconciliations()]);
      setRows(a.data.rows);
      setRecs(r.data.reconciliations);
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const reconcile = async (meterID) => {
    setBusy(meterID);
    try {
      const res = await api.metering.reconcile({ meterID, cycle });
      const r = res.data.reconciliation;
      toast(`${meterID}: ${r.status}. IoT ${num(r.iotKWh, 1)} kWh vs revenue meter ${num(r.utilityInjectedKWh, 1)} kWh (${pct(r.diffPct)}).`, r.status === 'RECONCILED' ? 'success' : 'error');
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
        title="Cycle audit"
        subtitle="IoT data vs. the utility revenue meter and vs. neighbors on the same substation (yield per installed kWp)"
        icon={<Gauge size={20} />}
        actions={
          <div className="flex items-end gap-2">
            <ScopeTag />
            <div className="w-32"><label className="label">Cycle</label><input className="input" value={cycle} onChange={(e) => setCycle(e.target.value)} /></div>
            <button className="btn-ghost" onClick={() => load(cycle)}><RefreshCw size={16} /></button>
          </div>
        }
      >
        {rows.length === 0 ? <Empty>No meters.</Empty> : (
          <div className="overflow-x-auto">
            <table className="min-w-full">
              <thead>
                <tr className="border-b">
                  <th className="th">Meter</th><th className="th">Days</th><th className="th">IoT energy</th><th className="th">Yield</th><th className="th">vs. neighbors</th>
                  <th className="th">Revenue meter</th><th className="th">Difference</th><th className="th">Status</th><th className="th">Signals</th><th className="th"></th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.meterID} className="border-b last:border-0 align-top">
                    <td className="td"><p className="font-mono text-xs">{r.meterID}</p><p className="text-xs text-slate-500">{r.owner} · {r.capacityKWp} kWp · {r.substation}</p></td>
                    <td className="td">{r.daysCovered}</td>
                    <td className="td">{kwh(r.iotKWh)}</td>
                    <td className="td">{num(r.iotYield)} kWh/kWp/d</td>
                    <td className="td">{r.peerRatio ? `${num(r.peerRatio)}× median (${num(r.peerMedianYield)})` : '—'}</td>
                    <td className="td">{r.reconStatus === 'PENDING' ? '—' : kwh(r.utilityInjectedKWh)}</td>
                    <td className="td">{r.reconStatus === 'PENDING' ? '—' : pct(r.diffPct)}</td>
                    <td className="td space-y-1"><Badge tone={reconTone(r.reconStatus)}>{r.reconStatus}</Badge><br /><Badge tone={meterTone(r.meterStatus)}>{r.meterStatus}</Badge></td>
                    <td className="td"><div className="flex flex-wrap gap-1">{r.flags.length === 0 ? <span className="text-xs text-slate-400">none</span> : r.flags.map((f) => <Badge key={f} tone="rose">{flagLabel[f] || f}</Badge>)}</div></td>
                    <td className="td text-right">
                      {r.reconStatus === 'PENDING' && (
                        <button className="btn-dark !py-1 text-xs" disabled={busy === r.meterID} onClick={() => reconcile(r.meterID)}>Read revenue meter & reconcile</button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="mt-4 text-xs text-slate-500">
          Official credits always come from the revenue meter. A difference above the tolerance does not change the credits. It sends the meter to physical inspection and quarantines its next readings. UtilityMSP only.
        </p>
      </Card>

      <Card title="Reconciliation records" subtitle="Immutable, endorsed by the consortium" icon={<Scale size={20} />}>
        {recs.length === 0 ? <Empty>No reconciliation yet.</Empty> : (
          <table className="min-w-full">
            <thead><tr className="border-b"><th className="th">Meter</th><th className="th">Cycle</th><th className="th">Batches</th><th className="th">IoT</th><th className="th">Revenue meter</th><th className="th">Consumed</th><th className="th">Status</th><th className="th">Note</th><th className="th">When</th></tr></thead>
            <tbody>
              {recs.map((r) => (
                <tr key={r.meterID + r.cycle} className="border-b last:border-0">
                  <td className="td font-mono text-xs">{r.meterID}</td>
                  <td className="td">{r.cycle}</td>
                  <td className="td">{r.readingsCount} ({r.flaggedCount} flagged)</td>
                  <td className="td">{kwh(r.iotKWh)}</td>
                  <td className="td">{kwh(r.utilityInjectedKWh)}</td>
                  <td className="td">{kwh(r.utilityConsumedKWh)}</td>
                  <td className="td"><Badge tone={reconTone(r.status)}>{r.status}</Badge></td>
                  <td className="td text-xs">{r.note}</td>
                  <td className="td text-xs text-slate-500">{time(r.reconciledAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </>
  );
};

export default Reconciliation;
