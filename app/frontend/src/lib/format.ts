import type { UsageTotals } from './types';

export function fmtTokens(n: number | undefined): string {
  n = n || 0;
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(n >= 10_000_000 ? 0 : 1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(n >= 10_000 ? 0 : 1) + 'k';
  return String(n);
}

export function fmtUsd(n: number | undefined): string {
  n = n || 0;
  if (n === 0) return '$0';
  if (n < 0.01) return '<$0.01';
  if (n < 100) return '$' + n.toFixed(2);
  return '$' + Math.round(n).toLocaleString();
}

export function usageLine(u: UsageTotals | undefined): string {
  if (!u || (!u.inputTokens && !u.outputTokens)) return '';
  const parts = [`${fmtTokens(u.inputTokens)} in`, `${fmtTokens(u.outputTokens)} out`];
  if (u.reasoningTokens) parts.push(`${fmtTokens(u.reasoningTokens)} thinking`);
  return parts.join(' · ') + ` · ${u.estimated ? '~' : ''}${fmtUsd(u.costUsd)}`;
}

export function relTime(iso: string | undefined): string {
  if (!iso) return '';
  const t = new Date(iso).getTime();
  if (!t) return '';
  const s = Math.round((Date.now() - t) / 1000);
  if (s < 45) return 'just now';
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24);
  if (d < 7) return `${d}d ago`;
  return new Date(iso).toLocaleDateString();
}

export function fmtDateTime(iso: string | undefined): string {
  if (!iso) return '-';
  const d = new Date(iso);
  if (!d.getTime()) return '-';
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

export function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function prettyJSON(v: unknown): string {
  if (v == null) return '';
  if (typeof v === 'string') {
    try {
      return JSON.stringify(JSON.parse(v), null, 2);
    } catch {
      return v;
    }
  }
  return JSON.stringify(v, null, 2);
}
