<script lang="ts">
  import { ChevronRight, Check, X, Loader2, Ban } from '@lucide/svelte';
  import type { Item } from '$lib/types';
  import { app } from '$lib/stores/app.svelte';
  import { prettyJSON } from '$lib/format';

  let { item }: { item: Item } = $props();
  let open = $state(false);

  const tool = $derived(item.tool!);
  const approval = $derived(app.approvals.find((a) => a.itemId === item.id));
  const args = $derived(prettyJSON(tool.args));
  const summary = $derived.by(() => {
    const a = tool.args as Record<string, unknown> | null;
    if (!a || typeof a !== 'object') return '';
    const v = a.command ?? a.path ?? a.skill_name ?? a.query ?? a.name ?? a.url;
    if (typeof v === 'string') return v.length > 80 ? v.slice(0, 80) + '…' : v;
    if (a.skill && a.script) return `${a.skill}/${a.script}`;
    return '';
  });
</script>

<div class="rounded-lg border {approval ? 'border-amber-500/40' : 'border-zinc-800'} bg-zinc-900/60 text-xs overflow-hidden">
  <button class="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-zinc-800/50" onclick={() => (open = !open)}>
    <ChevronRight class="w-3.5 h-3.5 text-zinc-500 transition-transform {open ? 'rotate-90' : ''}" />
    {#if item.status === 'inProgress'}
      <Loader2 class="w-3.5 h-3.5 text-violet-400 animate-spin" />
    {:else if item.status === 'completed'}
      <Check class="w-3.5 h-3.5 text-emerald-400" />
    {:else if item.status === 'denied'}
      <Ban class="w-3.5 h-3.5 text-amber-400" />
    {:else}
      <X class="w-3.5 h-3.5 text-red-400" />
    {/if}
    <code class="font-mono text-zinc-200">{tool.name}</code>
    {#if summary}<span class="text-zinc-500 truncate font-mono">{summary}</span>{/if}
    {#if tool.risk && tool.risk !== 'green'}<span class="ml-auto risk-{tool.risk}">{tool.risk}</span>{/if}
  </button>

  {#if approval}
    <div class="px-3 py-2 border-t border-amber-500/30 bg-amber-500/5 flex items-center gap-2">
      <span class="text-amber-200 flex-1 selectable">{approval.actionSummary || `Run ${approval.tool}`}{approval.reason ? ` — ${approval.reason}` : ''}</span>
      <button class="btn-outline btn-sm" onclick={() => app.respondApproval(approval.id, false)}>Deny</button>
      <button class="btn-primary btn-sm" onclick={() => app.respondApproval(approval.id, true)}>Approve</button>
    </div>
  {/if}

  {#if open}
    <div class="border-t border-zinc-800 divide-y divide-zinc-800">
      {#if args && args !== '{}'}
        <div class="px-3 py-2">
          <div class="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 mb-1">Arguments</div>
          <pre class="font-mono text-[11px] text-zinc-300 whitespace-pre-wrap break-all max-h-64 overflow-auto">{args}</pre>
        </div>
      {/if}
      {#if tool.output}
        <div class="px-3 py-2">
          <div class="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 mb-1">Output</div>
          <pre class="font-mono text-[11px] text-zinc-400 whitespace-pre-wrap break-all max-h-80 overflow-auto">{tool.output}</pre>
        </div>
      {/if}
      {#if tool.error}
        <div class="px-3 py-2 text-red-300 selectable">{tool.error}</div>
      {/if}
    </div>
  {:else if tool.error && item.status !== 'inProgress'}
    <div class="px-3 pb-1.5 text-[11px] {item.status === 'denied' ? 'text-amber-300/80' : 'text-red-300/80'} truncate">{tool.error}</div>
  {/if}
</div>
