<script lang="ts">
  import { X, FileText, GitCompare, Terminal, Undo2, RefreshCw } from '@lucide/svelte';
  import { inspector } from '$lib/stores/inspector.svelte';
  import DiffView from './DiffView.svelte';

  const tab = $derived(inspector.active);

  const icon = { file: FileText, diff: GitCompare, output: Terminal } as const;

</script>

<div class="h-full flex flex-col bg-surface/40">
  <header class="h-12 shrink-0 border-b border-line flex items-stretch px-1 gap-1 overflow-x-auto">
    {#each inspector.tabs as t (t.id)}
      {@const on = inspector.activeId === t.id}
      {@const Icon = icon[t.kind]}
      <div class="flex items-center gap-1 px-2 my-1.5 rounded-md text-xs {on ? 'bg-raised text-ink' : 'text-muted hover:bg-raised/60'}">
        <button class="flex items-center gap-1.5 max-w-44 truncate" onclick={() => (inspector.activeId = t.id)}>
          <Icon class="w-3.5 h-3.5 shrink-0" />
          <span class="truncate">{t.title}</span>
        </button>
        <button class="text-faint hover:text-ink" aria-label="Close tab" onclick={() => inspector.close(t.id)}>
          <X class="w-3 h-3" />
        </button>
      </div>
    {/each}
    <button class="btn-ghost btn-sm ml-auto my-1.5 shrink-0" aria-label="Close inspector" onclick={() => inspector.closeAll()}>
      <X class="w-3.5 h-3.5" />
    </button>
  </header>

  {#if tab}
    <div class="px-3 py-1.5 border-b border-line flex items-center gap-2 text-[11px] text-muted">
      <span class="font-mono truncate selectable">{tab.path ?? tab.title}</span>
      <div class="ml-auto flex items-center gap-1 shrink-0">
        {#if tab.kind === 'diff' && tab.change?.turnId && tab.change.revertable}
          <button class="btn-ghost btn-sm" onclick={() => inspector.revertTurn(tab.change!.turnId!, [tab.change!.path])}>
            <Undo2 class="w-3.5 h-3.5" />Undo
          </button>
        {/if}
        {#if tab.kind === 'file' && tab.path}
          <button class="btn-ghost btn-sm" aria-label="Reload" onclick={() => inspector.openFile(tab.path!)}>
            <RefreshCw class="w-3.5 h-3.5" />
          </button>
        {/if}
      </div>
    </div>

    <div class="flex-1 min-h-0 overflow-auto">
      {#if tab.error}
        <p class="p-4 text-sm text-rust">{tab.error}</p>
      {:else if tab.loading}
        <p class="p-4 text-sm text-muted">Loading…</p>
      {:else if tab.kind === 'diff' && tab.change}
        <div class="p-3">
          <p class="text-[11px] text-muted mb-2">
            {tab.change.action} · <span class="text-sage">+{tab.change.additions}</span>
            <span class="text-rust">−{tab.change.deletions}</span>
          </p>
          <DiffView change={tab.change} />
        </div>
      {:else if tab.kind === 'file'}
        {#if tab.binary}
          <p class="p-4 text-sm text-muted">This file is not text, so there is nothing to show here.</p>
        {:else}
          <pre class="p-3 font-mono text-[11px] leading-[1.5] whitespace-pre-wrap selectable">{tab.content}</pre>
          {#if tab.truncated}<p class="px-3 pb-3 text-[11px] text-muted">… only the first part of this file is shown.</p>{/if}
        {/if}
      {:else}
        <pre class="p-3 font-mono text-[11px] leading-[1.5] whitespace-pre-wrap selectable">{tab.text}</pre>
      {/if}
    </div>
  {/if}
</div>
