import React, { useEffect, useState } from 'react';
import { FlaskConical, Lock } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useToast } from '../components/Toast';
import { Card, Badge, Empty, ScopeTag, statusTone } from '../components/ui';
import { num, rate, brl, time } from '../services/format';

const Sandbox = () => {
  const toast = useToast();
  const [cfg, setCfg] = useState(null);
  const [band, setBand] = useState(null);
  const [orders, setOrders] = useState([]);
  const [trades, setTrades] = useState([]);
  const [users, setUsers] = useState([]);
  const [reference, setReference] = useState('');
  const [form, setForm] = useState({ side: 'ASK', owner: '', quantity: '10', priceBRL: '0.62' });

  const load = async () => {
    try {
      const [c, b, o, t, u] = await Promise.all([api.governance.getSandbox(), api.governance.getPriceBand(), api.sandbox.listOrders(), api.sandbox.listTrades(), api.users.list()]);
      setCfg(c.data.sandbox);
      setBand(b.data.priceBand);
      setOrders(o.data.orders);
      setTrades(t.data.trades);
      setUsers(u.data.users);
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const toggle = async (enabled) => {
    try {
      await api.governance.setSandbox({ enabled, reference: enabled ? reference : '' });
      toast(enabled ? 'Sandbox simulation enabled.' : 'Sandbox disabled. P2P trading is blocked again.');
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };

  const submit = async (e) => {
    e.preventDefault();
    const payload = { owner: form.owner, substation: 'SUB-01', quantity: parseFloat(form.quantity), priceBRL: parseFloat(form.priceBRL) };
    try {
      const res = form.side === 'ASK' ? await api.sandbox.submitAsk(payload) : await api.sandbox.submitBid(payload);
      toast(`Order placed (${res.data.result.trades.length} simulated trade(s)).`);
      load();
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };

  const live = orders.filter((o) => o.status === 'OPEN' || o.status === 'PARTIAL');

  return (
    <>
      <div className="rounded-2xl border border-violet-200 bg-violet-50 p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="flex items-center gap-2 text-base font-semibold text-violet-900"><Lock size={18} /> Legally dependent capability, disabled by default</p>
          <ScopeTag future />
        </div>
        <p className="mt-2 text-sm text-violet-900">
          Under Lei 14.300 (art. 28) and REN ANEEL 1.059/2023, surplus energy and credits can only offset consumption of the same holder in the same distribution area. Selling or transferring credits to a neighbor is not allowed today. This module exists to test the market design inside an ANEEL regulatory sandbox and only runs when the utility enables it with an authorization reference. All money values are simulated.
        </p>
      </div>

      <div className="grid gap-6 xl:grid-cols-3">
        <Card title="Sandbox switch" subtitle="governance-cc · UtilityMSP only" icon={<FlaskConical size={20} />}>
          {cfg && (
            <div className="space-y-3 text-sm">
              <p>Status {cfg.p2pEnabled ? <Badge tone="violet">ENABLED (simulation)</Badge> : <Badge>DISABLED</Badge>}</p>
              {cfg.reference && <p className="text-slate-500">Reference {cfg.reference}</p>}
              {cfg.p2pEnabled ? (
                <button className="btn-dark w-full" onClick={() => toggle(false)}>Disable sandbox</button>
              ) : (
                <>
                  <input className="input" placeholder="Authorization reference (required)" value={reference} onChange={(e) => setReference(e.target.value)} />
                  <button className="btn-ghost w-full" onClick={() => toggle(true)}>Enable sandbox simulation</button>
                </>
              )}
              {band && <p className="text-xs text-slate-500">Corridor {rate(band.floorBRL)} to {rate(band.ceilingBRL)}, derived from the tariffs (credit value + R$0.01 up to the full tariff − R$0.02).</p>}
            </div>
          )}
        </Card>
        <Card title="Try an order" subtitle="Refused by the chaincode while the sandbox is off" className="xl:col-span-2">
          <form onSubmit={submit} className="grid gap-3 md:grid-cols-5">
            <select className="input" value={form.side} onChange={(e) => setForm({ ...form, side: e.target.value })}><option value="ASK">Sell (ask)</option><option value="BID">Buy (bid)</option></select>
            <select className="input" value={form.owner} onChange={(e) => setForm({ ...form, owner: e.target.value })} required>
              <option value="">Holder…</option>
              {users.map((u) => <option key={u.id} value={u.id}>{u.displayName}</option>)}
            </select>
            <input className="input" type="number" step="0.1" value={form.quantity} onChange={(e) => setForm({ ...form, quantity: e.target.value })} />
            <input className="input" type="number" step="0.01" value={form.priceBRL} onChange={(e) => setForm({ ...form, priceBRL: e.target.value })} />
            <button className="btn-dark">Submit</button>
          </form>
          <div className="mt-4 grid gap-4 md:grid-cols-2">
            <div>
              <p className="mb-2 text-sm font-semibold">Open orders</p>
              {live.length === 0 ? <Empty>Empty</Empty> : live.map((o) => (
                <p key={o.orderID} className="flex justify-between border-b py-1 text-sm"><span>{o.side} {num(o.remaining)} kWh @ {rate(o.priceBRL)}</span><span>{o.owner} <Badge tone={statusTone(o.status)}>{o.status}</Badge></span></p>
              ))}
            </div>
            <div>
              <p className="mb-2 text-sm font-semibold">Simulated trades</p>
              {trades.length === 0 ? <Empty>None</Empty> : trades.map((t) => (
                <p key={t.tradeID} className="flex justify-between border-b py-1 text-sm"><span>{t.seller} → {t.buyer} · {num(t.quantity)} kWh</span><span>{brl(t.valueBRL)} · {time(t.executedAt)}</span></p>
              ))}
            </div>
          </div>
        </Card>
      </div>
    </>
  );
};

export default Sandbox;
