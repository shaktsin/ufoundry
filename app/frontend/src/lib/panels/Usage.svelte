<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { fmtTokens, fmtUsd } from '$lib/format';
  import type { UsageSummary } from '$lib/types';

  type Range = 'month' | '7d' | '30d' | 'today';
  let range = $state<Range>('month');
  let groupBy = $state<'credential' | 'model' | 'thread' | 'day' | 'role'>('credential');
  let data = $state<UsageSummary | null>(null);

  function bounds(r: Range): { from: string; to: string } {
    const now = new Date();
    const to = new Date(now.getTime() + 60_000);
    let from: Date;
    if (r === 'month') from = new Date(now.getFullYear(), now.getMonth(), 1);
    else if (r === 'today') from = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    else from = new Date(now.getTime() - (r === '7d' ? 7 : 30) * 86_400_000);
    return { from: from.toISOString(), to: to.toISOString() };
  }

  async function load() {
    const r = await app.try<UsageSummary>('usage/summary', { groupBy, ...bounds(range) });
    if (r) data = r;
  }

  $effect(() => {
    void range;
    void groupBy;
    if (app.conn === 'open') void load();
  });

  const maxCost = $derived(Math.max(0.000001, ...(data?.rows ?? []).map((r) => r.usage.costUsd)));
  const budgets = $derived(app.credentials.filter((c) => (c.monthlyBudgetUsd ?? 0) > 0));
</script>

<div class="flex items-center gap-2 mb-4">
  <h1 class="page-title mr-2">Usage</h1>
  <select class="select" bind:value={range}>
    <option value="today">Today</option><option value="7d">Last 7 days</option><option value="month">This month</option><option value="30d">Last 30 days</option>
  </select>
  <select class="select" bind:value={groupBy}>
    <option value="credential">By API key</option><option value="model">By model</option><option value="thread">By chat</option><option value="day">By day</option><option value="role">By role</option>
  </select>
</div>

{#if data}
  <div class="grid grid-cols-4 gap-3 mb-5">
    {#each [
      ['Cost', (data.total.estimated ? '~' : '') + fmtUsd(data.total.costUsd)],
      ['Input tokens', fmtTokens(data.total.inputTokens)],
      ['Output tokens', fmtTokens(data.total.outputTokens)],
      ['Requests', String(data.total.requests)],
    ] as [label, value]}
      <div class="card p-3"><div class="text-xs text-zinc-500">{label}</div><div class="text-xl font-semibold mt-0.5">{value}</div></div>
    {/each}
  </div>

  {#if budgets.length}
    <h2 class="text-sm font-semibold text-zinc-300 mb-2">Monthly budgets</h2>
    <div class="grid grid-cols-2 gap-3 mb-5">
      {#each budgets as c (c.id)}
        {@const spent = c.monthUsage?.costUsd ?? 0}
        {@const pct = Math.min(100, (spent / (c.monthlyBudgetUsd || 1)) * 100)}
        <div class="card p-3">
          <div class="flex text-sm"><span>{c.label}</span><span class="ml-auto text-zinc-400">{fmtUsd(spent)} / {fmtUsd(c.monthlyBudgetUsd)}</span></div>
          <div class="h-1.5 bg-zinc-800 rounded mt-2 overflow-hidden">
            <div class="h-full {pct >= 100 ? 'bg-red-500' : pct >= 80 ? 'bg-amber-500' : 'bg-violet-500'}" style="width: {pct}%"></div>
          </div>
          <div class="text-[11px] text-zinc-500 mt-1">{c.hardStop ? 'Stops at 100%' : 'Warns at 80% and 100%'}</div>
        </div>
      {/each}
    </div>
  {/if}

  {#if data.rows.length === 0}
    <div class="card p-8 text-center text-sm text-zinc-500">No usage in this period.</div>
  {:else}
    <div class="card overflow-hidden">
      <table class="data-table">
        <thead><tr><th>{groupBy === 'credential' ? 'API key' : groupBy === 'thread' ? 'Chat' : groupBy[0].toUpperCase() + groupBy.slice(1)}</th><th class="w-1/3"></th><th class="text-right">In</th><th class="text-right">Out</th><th class="text-right">Requests</th><th class="text-right">Cost</th></tr></thead>
        <tbody>
          {#each data.rows as r (r.key)}
            <tr>
              <td class="text-zinc-200">
                {#if groupBy === 'thread'}
                  <button class="hover:underline text-left" onclick={() => chat.open(r.key)}>{r.label || r.key}</button>
                {:else}{r.label || r.key}{/if}
              </td>
              <td><div class="h-1.5 bg-violet-500/70 rounded" style="width: {(r.usage.costUsd / maxCost) * 100}%"></div></td>
              <td class="text-right text-zinc-400">{fmtTokens(r.usage.inputTokens)}</td>
              <td class="text-right text-zinc-400">{fmtTokens(r.usage.outputTokens)}</td>
              <td class="text-right text-zinc-400">{r.usage.requests}</td>
              <td class="text-right">{r.usage.estimated ? '~' : ''}{fmtUsd(r.usage.costUsd)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="text-[11px] text-zinc-600 mt-2">Costs use the model prices in Settings → Models. “~” means some requests used estimated token counts or unknown prices.</p>
  {/if}
{/if}
