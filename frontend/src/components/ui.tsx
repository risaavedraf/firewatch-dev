import type { ReactNode } from 'react';

// Presentation primitives only. They decide how something looks, never whether
// it is true: every primitive here renders whatever it is handed, and the
// decisions about validity, permission and honesty live in the domain layer and
// in the views that use them.

export type IconName =
  | 'territory'
  | 'history'
  | 'sources'
  | 'pipeline'
  | 'jobs'
  | 'session'
  | 'audit'
  | 'permissions'
  | 'pending'
  | 'actions'
  | 'search'
  | 'refresh'
  | 'zones'
  | 'batches'
  | 'attention';

// The icon set the page already shipped. Geometric paths, no icon font and no
// external asset, so the strict `script-src 'self'`/`img-src 'self'` policy is
// unaffected by navigation.
const ICONS: Record<IconName, ReactNode> = {
  territory: <path d="m3 5 6-2 6 2 6-2v16l-6 2-6-2-6 2V5Zm6-2v16m6-14v16" />,
  history: <path d="M3 11a9 9 0 1 1 2 7M3 4v7h7m2-5v6l4 2" />,
  sources: <path d="m12 3 10 5-10 5L2 8l10-5Zm-10 9 10 5 10-5M2 16l10 5 10-5" />,
  pipeline: <path d="m5 12 4 4L19 6" />,
  jobs: <path d="M2 12h5l3-8 4 16 3-8h5" />,
  session: (
    <>
      <rect x="5" y="10" width="14" height="11" rx="2" />
      <path d="M8 10V6a4 4 0 0 1 8 0v4m-4 5v2" />
    </>
  ),
  audit: (
    <>
      <rect x="4" y="4" width="16" height="17" rx="2" />
      <path d="M8 9h8M8 13h8M8 17h5" />
    </>
  ),
  permissions: (
    <>
      <path d="M12 3l7 3v6c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6l7-3Z" />
      <path d="M9 12l2 2 4-4" />
    </>
  ),
  pending: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v6m0 4h.01" />
    </>
  ),
  search: (
    <>
      <circle cx="10" cy="10" r="6" />
      <path d="m15 15 6 6" />
    </>
  ),
  refresh: <path d="M4 10a8 8 0 1 1 1 8M4 3v7h7" />,
  actions: <path d="M9 5h10M9 12h10M9 19h10M4 5l1 1 2-2M4 12l1 1 2-2M4 19l1 1 2-2" />,
  zones: <path d="m3 5 6-2 6 2 6-2v16l-6 2-6-2-6 2V5Zm6-2v16m6-14v16" />,
  batches: <path d="M3 11a9 9 0 1 1 2 7M3 4v7h7m2-5v6l4 2" />,
  attention: <path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Zm-4 9 3 3 5-6" />,
};

export function Icon({ name, className }: { name: IconName; className?: string }) {
  return (
    <svg
      className={className ? `icon ${className}` : 'icon'}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {ICONS[name]}
    </svg>
  );
}

// Tones map to the level and state palettes declared in `styles/app.css`. The
// caller chooses the tone from a canonical domain value, never from a string it
// guessed, so a colour can never claim a level the payload did not state.
export type ChipTone = 'neutral' | 'high' | 'medium' | 'low' | 'info';

export function Chip({ tone = 'neutral', children }: { tone?: ChipTone; children: ReactNode }) {
  return (
    <span className={tone === 'neutral' ? 'chip' : `chip chip-${tone}`}>
      <span className="chip-dot" aria-hidden="true" />
      {children}
    </span>
  );
}

export function Metric({
  label,
  icon,
  value,
  note,
  attention = false,
}: {
  label: string;
  icon: IconName;
  value: ReactNode;
  note: ReactNode;
  attention?: boolean;
}) {
  return (
    <article className={attention ? 'metric metric-attention' : 'metric'}>
      <div className="metric-top">
        <span>{label}</span>
        <Icon name={icon} />
      </div>
      <strong>{value}</strong>
      <p>{note}</p>
    </article>
  );
}

export function PanelHeading({
  eyebrow,
  title,
  titleId,
  badge,
}: {
  eyebrow: string;
  title: string;
  titleId?: string;
  badge?: ReactNode;
}) {
  return (
    <div className="panel-heading">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h3 id={titleId}>{title}</h3>
      </div>
      {badge}
    </div>
  );
}

export function DataList({
  items,
  compact = false,
}: {
  items: ReadonlyArray<{ label: string; value: ReactNode }>;
  compact?: boolean;
}) {
  return (
    <dl className={compact ? 'data-list compact' : 'data-list'}>
      {items.map(item => (
        <div key={item.label}>
          <dt>{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

// Live region used for every panel that reports progress or failure, so a screen
// reader hears the state change instead of a silently repainted paragraph.
export function StatusCopy({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <p id={id} className="status-copy" role="status" aria-live="polite">
      {children}
    </p>
  );
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <div className="empty-state">{children}</div>;
}

// The honest marker for a capability the backend cannot back yet. It exists so
// the page can name what is missing; it never wraps an action that would fake it.
export function UnimplementedNote({ children }: { children: ReactNode }) {
  return (
    <p className="unimplemented-note">
      <span className="unimplemented">NO IMPLEMENTADO</span> {children}
    </p>
  );
}

export function UnimplementedBadge() {
  return <span className="unimplemented">NO IMPLEMENTADO</span>;
}
