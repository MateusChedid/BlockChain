import React from 'react';

export const Card = ({ title, subtitle, icon, actions, children, className = '' }) => (
  <section className={`card ${className}`}>
    {(title || actions) && (
      <div className="mb-4 flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          {icon && <div className="rounded-xl bg-emerald-50 p-2 text-emerald-600">{icon}</div>}
          <div>
            {title && <h3 className="text-base font-semibold text-slate-900">{title}</h3>}
            {subtitle && <p className="text-sm text-slate-500">{subtitle}</p>}
          </div>
        </div>
        {actions}
      </div>
    )}
    {children}
  </section>
);

export const Stat = ({ label, value, hint, icon, tone = 'emerald' }) => {
  const tones = {
    emerald: 'bg-emerald-50 text-emerald-600',
    amber: 'bg-amber-50 text-amber-600',
    sky: 'bg-sky-50 text-sky-600',
    violet: 'bg-violet-50 text-violet-600',
    rose: 'bg-rose-50 text-rose-600',
    slate: 'bg-slate-100 text-slate-600',
  };
  return (
    <div className="card flex items-start justify-between">
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</p>
        <p className="mt-2 text-2xl font-bold text-slate-900">{value}</p>
        {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
      </div>
      {icon && <div className={`rounded-xl p-2 ${tones[tone]}`}>{icon}</div>}
    </div>
  );
};

export const Badge = ({ children, tone = 'slate' }) => {
  const tones = {
    slate: 'bg-slate-100 text-slate-700',
    emerald: 'bg-emerald-100 text-emerald-700',
    amber: 'bg-amber-100 text-amber-800',
    rose: 'bg-rose-100 text-rose-700',
    sky: 'bg-sky-100 text-sky-700',
    violet: 'bg-violet-100 text-violet-700',
  };
  return <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold ${tones[tone]}`}>{children}</span>;
};

export const statusTone = (status) =>
  ({ OPEN: 'sky', PARTIAL: 'amber', FILLED: 'emerald', CANCELLED: 'slate' }[status] || 'slate');

export const Empty = ({ children }) => <p className="py-6 text-center text-sm text-slate-400">{children}</p>;

export const reconTone = (status) =>
  ({ RECONCILED: 'emerald', DISCREPANCY: 'rose', PENDING: 'slate' }[status] || 'slate');

export const meterTone = (status) =>
  ({ OK: 'emerald', FLAGGED: 'amber', INSPECTION_REQUIRED: 'rose' }[status] || 'slate');

export const flagLabel = {
  IMPLAUSIBLE_READING: 'Implausible reading',
  REVENUE_METER_MISMATCH: 'Revenue meter mismatch',
  INSPECTION_REQUIRED: 'Inspection required',
  ABOVE_PEERS: 'Above neighbors',
  UNDERPERFORMING: 'Underperforming',
};

// Marks what the MVP demonstrates versus what depends on regulation.
export const ScopeTag = ({ future }) =>
  future ? (
    <span className="inline-flex items-center rounded-full bg-violet-100 px-2 py-0.5 text-xs font-semibold text-violet-700">FUTURE · depends on regulation</span>
  ) : (
    <span className="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-700">DEMONSTRATED</span>
  );

export const SimulatedTag = () => (
  <span className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-semibold text-amber-800">SIMULATED · not a real bill</span>
);
