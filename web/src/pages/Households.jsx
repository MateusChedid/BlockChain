import React, { useEffect, useState } from 'react';
import { UserPlus } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Badge, Empty } from '../components/ui';

const Households = () => {
  const toast = useToast();
  const [users, setUsers] = useState([]);
  const [form, setForm] = useState({ id: '', displayName: '', type: 'prosumer', substation: 'SUB-01', consumerUnit: '' });

  const load = () => api.users.list().then((r) => setUsers(r.data.users)).catch((e) => toast(errorMessage(e), 'error'));
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const submit = async (e) => {
    e.preventDefault();
    try {
      await api.users.create(form);
      toast(`Household ${form.displayName} created.`);
      setForm({ ...form, id: '', displayName: '', consumerUnit: '' });
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };

  return (
    <>
      <Card title="New household" subtitle="Personal data stays off-chain (LGPD). Only the holder ID is used on the ledger." icon={<UserPlus size={20} />}>
        <form onSubmit={submit} className="grid gap-3 md:grid-cols-6">
          <input className="input" placeholder="Holder ID (e.g. alice)" value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })} required />
          <input className="input" placeholder="Display name" value={form.displayName} onChange={(e) => setForm({ ...form, displayName: e.target.value })} required />
          <select className="input" value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
            <option value="prosumer">prosumer (has solar panels)</option>
            <option value="consumer">consumer</option>
          </select>
          <input className="input" placeholder="Substation" value={form.substation} onChange={(e) => setForm({ ...form, substation: e.target.value })} required />
          <input className="input" placeholder="Consumer unit (UC)" value={form.consumerUnit} onChange={(e) => setForm({ ...form, consumerUnit: e.target.value })} />
          <button className="btn-dark">Create household</button>
        </form>
      </Card>
      <Card title="Registered households">
        {users.length === 0 ? <Empty>No households yet.</Empty> : (
          <table className="min-w-full">
            <thead><tr className="border-b"><th className="th">Holder ID</th><th className="th">Name</th><th className="th">Type</th><th className="th">Substation</th><th className="th">UC</th></tr></thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} className="border-b last:border-0">
                  <td className="td font-mono text-xs">{u.id}</td>
                  <td className="td">{u.displayName}</td>
                  <td className="td"><Badge tone={u.type === 'prosumer' ? 'amber' : 'sky'}>{u.type}</Badge></td>
                  <td className="td"><Badge>{u.substation}</Badge></td>
                  <td className="td">{u.consumerUnit || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </>
  );
};

export default Households;
