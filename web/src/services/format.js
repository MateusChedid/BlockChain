export const kwh = (v) => `${Number(v || 0).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} kWh`;
export const num = (v, d = 2) => Number(v || 0).toLocaleString('pt-BR', { maximumFractionDigits: d });
export const brl = (v) => new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(Number(v || 0));
export const rate = (v) => `R$ ${Number(v || 0).toLocaleString('pt-BR', { minimumFractionDigits: 4, maximumFractionDigits: 5 })}/kWh`;
export const pct = (v) => `${Number(v || 0) > 0 ? '+' : ''}${Number(v || 0).toLocaleString('pt-BR', { maximumFractionDigits: 2 })}%`;
export const time = (ts) => (ts ? new Date(ts * 1000).toLocaleString('pt-BR') : '—');
export const shortId = (id = '') => (id.length > 14 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id);
export const currentCycle = () => new Date().toISOString().slice(0, 7);
export const TREASURY = 'wattledger-treasury';
