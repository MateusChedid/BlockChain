import React, { useEffect, useState } from 'react';
import { Settings2, Building2 } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Badge, Empty } from '../components/ui';
import { rate, time } from '../services/format';

const Governance = () => {
  const toast = useToast();
  const [t, setT] = useState(null);
  const [members, setMembers] = useState([]);
  const [form, setForm] = useState(null);
  const [member, setMember] = useState({ memberID: '', mspID: 'UtilityMSP', displayName: '', role: 'utility' });
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const [tr, m] = await Promise.all([api.governance.getTariffs(), api.governance.listMembers()]);
      setT(tr.data.tariffs);
      setMembers(m.data.members);
      const x = tr.data.tariffs;
      setForm({ distributor: x.distributor, teBRL: String(x.teBRL), tusdBRL: String(x.tusdBRL), fioBBRL: String(x.fioBBRL), fioBPct: String(x.fioBPct), availabilityKWh: String(x.availabilityKWh), tolerancePct: String(x.tolerancePct) });
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.governance.updateTariffs({
        distributor: form.distributor, teBRL: parseFloat(form.teBRL), tusdBRL: parseFloat(form.tusdBRL), fioBBRL: parseFloat(form.fioBBRL),
        fioBPct: parseInt(form.fioBPct, 10), availabilityKWh: parseFloat(form.availabilityKWh), tolerancePct: parseFloat(form.tolerancePct),
      });
      toast('Tariffs updated on-chain.');
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  const onboard = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.governance.onboardMember(member);
      toast(`Member ${member.memberID} onboarded.`);
      setMember({ ...member, memberID: '', displayName: '' });
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  const field = (key, label, step = '0.00001') => (
    <div><label className="label">{label}</label><input className="input" type={key === 'distributor' ? 'text' : 'number'} step={step} value={form[key]} onChange={(e) => setForm({ ...form, [key]: e.target.value })} /></div>
  );

  return (
    <>
      <Card title="Reference tariffs" subtitle="Used by the simulated bill and by the reconciliation tolerance. Only UtilityMSP can change them." icon={<Settings2 size={20} />}>
        <div className="grid gap-6 lg:grid-cols-3">
          {t && (
            <div className="space-y-3 lg:col-span-2">
              <div className="grid grid-cols-2 gap-3 text-sm md:grid-cols-3">
                <div className="rounded-xl bg-slate-50 p-3"><p className="label">TE + TUSD</p><p className="font-semibold">{rate(t.teBRL + t.tusdBRL)}</p></div>
                <div className="rounded-xl bg-slate-50 p-3"><p className="label">Fio B not compensated</p><p className="font-semibold">{t.fioBPct}% × {rate(t.fioBBRL)}</p></div>
                <div className="rounded-xl bg-slate-50 p-3"><p className="label">Value of 1 kWh of credit</p><p className="font-semibold">{rate(t.teBRL + t.tusdBRL - (t.fioBBRL * t.fioBPct) / 100)}</p></div>
                <div className="rounded-xl bg-slate-50 p-3"><p className="label">Minimum billed</p><p className="font-semibold">{t.availabilityKWh} kWh</p></div>
                <div className="rounded-xl bg-slate-50 p-3"><p className="label">Reconciliation tolerance</p><p className="font-semibold">±{t.tolerancePct}%</p></div>
                <div className="rounded-xl bg-slate-50 p-3"><p className="label">Updated by</p><p className="font-semibold">{t.updatedBy}</p><p className="text-xs text-slate-500">{time(t.updatedAt)}</p></div>
              </div>
              <p className="text-xs text-slate-500">
                TE and TUSD from Celesc B1 residential bills of late 2025 (before taxes). The Fio B value is an assumption to be replaced by the utility. Fio B schedule from Lei 14.300 art. 27 (60% in 2026, 75% in 2027, 90% in 2028).
              </p>
            </div>
          )}
          {form && (
            <form onSubmit={save} className="space-y-3 rounded-xl border border-slate-100 p-4">
              {field('distributor', 'Distributor')}
              <div className="grid grid-cols-2 gap-2">{field('teBRL', 'TE R$/kWh')}{field('tusdBRL', 'TUSD R$/kWh')}</div>
              <div className="grid grid-cols-2 gap-2">{field('fioBBRL', 'Fio B R$/kWh')}{field('fioBPct', 'Fio B %', '1')}</div>
              <div className="grid grid-cols-2 gap-2">{field('availabilityKWh', 'Minimum kWh', '1')}{field('tolerancePct', 'Tolerance %', '0.5')}</div>
              <button className="btn-primary w-full" disabled={busy}>Submit UpdateTariffs</button>
            </form>
          )}
        </div>
      </Card>

      <Card title="Consortium members" subtitle="Onboarding is restricted to ConsortiumMSP" icon={<Building2 size={20} />}>
        <form onSubmit={onboard} className="mb-5 grid gap-3 md:grid-cols-5">
          <input className="input" placeholder="Member ID" value={member.memberID} onChange={(e) => setMember({ ...member, memberID: e.target.value })} required />
          <select className="input" value={member.mspID} onChange={(e) => setMember({ ...member, mspID: e.target.value })}>
            <option>UtilityMSP</option><option>InstallerMSP</option><option>ConsortiumMSP</option>
          </select>
          <input className="input" placeholder="Display name" value={member.displayName} onChange={(e) => setMember({ ...member, displayName: e.target.value })} />
          <select className="input" value={member.role} onChange={(e) => setMember({ ...member, role: e.target.value })}>
            <option value="utility">utility</option><option value="installer">installer</option><option value="operator">operator</option>
          </select>
          <button className="btn-dark" disabled={busy}>Onboard member</button>
        </form>
        {members.length === 0 ? <Empty>No members onboarded yet.</Empty> : (
          <table className="min-w-full">
            <thead><tr className="border-b"><th className="th">Member</th><th className="th">Name</th><th className="th">MSP</th><th className="th">Role</th><th className="th">Onboarded</th></tr></thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.memberID} className="border-b last:border-0">
                  <td className="td font-mono text-xs">{m.memberID}</td>
                  <td className="td">{m.displayName}</td>
                  <td className="td"><Badge tone="emerald">{m.mspID}</Badge></td>
                  <td className="td">{m.role}</td>
                  <td className="td text-xs text-slate-500">{time(m.createdAt)} by {m.onboardedBy}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </>
  );
};

export default Governance;
