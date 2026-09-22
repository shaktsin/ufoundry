<script lang="ts">
  import { tick } from 'svelte';
  import { Sparkles, KeyRound } from '@lucide/svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { usageLine, fmtUsd } from '$lib/format';
  import ItemView from '$lib/components/ItemView.svelte';
  import Composer from '$lib/components/Composer.svelte';
  import type { Item, Turn } from '$lib/types';

  let scroller: HTMLDivElement | undefined = $state();
  let stick = true;

  interface Group { turnId: string; turn?: Turn; items: Item[] }

  const groups = $derived.by(() => {
    const out: Group[] = [];
    const byId = new Map<string, Group>();
    for (const it of chat.items) {
      let g = byId.get(it.turnId);
      if (!g) {
        g = { turnId: it.turnId, turn: chat.turns.find((t) => t.id === it.turnId), items: [] };
        byId.set(it.turnId, g);
        out.push(g);
      }
      g.items.push(it);
    }
    // A running turn with no items yet still shows its spinner.
    for (const t of chat.turns) {
      if (!byId.has(t.id)) out.push({ turnId: t.id, turn: t, items: [] });
    }
    return out;
  });

  const hasKeys = $derived(app.credentials.some((c) => c.enabled));

  function onScroll() {
    if (!scroller) return;
    stick = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 80;
  }

  // Follow new output while the user is at the bottom.
  $effect(() => {
    void chat.items.length;
    void chat.items.at(-1)?.text;
    void chat.turns.length;
    if (stick) tick().then(() => scroller && (scroller.scrollTop = scroller.scrollHeight));
  });

  // Jump to a search hit.
  $effect(() => {
    const id = chat.highlightItem;
    if (!id || chat.loading) return;
    tick().then(() => {
      document.getElementById(`item-${id}`)?.scrollIntoView({ block: 'center' });
      stick = false;
      setTimeout(() => (chat.highlightItem = null), 2500);
    });
  });

  $effect(() => {
    void chat.activeId;
    stick = true;
  });

  function modelLabel(t: Turn): string {
    const r = t.resolved || {};
    const m = app.models.find((x) => x.provider === r.provider && x.id === r.model);
    const name = m?.displayName || r.model || '';
    const cx = r.complexity ? `${r.complexity}${t.autoPicked ? ' (auto)' : ''}` : '';
    return [name, cx].filter(Boolean).join(' · ');
  }
</script>

<div class="flex-1 min-h-0 flex flex-col">
  <header class="h-12 shrink-0 border-b border-zinc-800 flex items-center px-4 gap-3">
    <h1 class="text-sm font-medium truncate">{chat.active?.title || (chat.activeId ? 'Untitled' : 'New chat')}</h1>
    {#if chat.active && chat.active.usage?.requests}
      <span class="ml-auto text-[11px] text-zinc-500" title="Tokens and cost for this chat">
        {fmtUsd(chat.active.usage.costUsd)} · {chat.active.usage.requests} requests
      </span>
    {/if}
  </header>

  <div class="flex-1 min-h-0 overflow-y-auto" bind:this={scroller} onscroll={onScroll}>
    {#if !chat.activeId}
      <div class="h-full flex flex-col items-center justify-center text-center px-6">
        <Sparkles class="w-8 h-8 text-violet-400 mb-3" />
        <h2 class="text-lg font-semibold">What can I help with?</h2>
        {#if app.conn === 'open' && !hasKeys}
          <p class="text-sm text-zinc-400 mt-2 max-w-sm">Add an API key for Claude, OpenAI, Gemini or a local model server to get started.</p>
          <button class="btn-primary mt-4" onclick={() => (app.view = 'settings')}><KeyRound class="w-4 h-4" />Add an API key</button>
        {:else}
          <p class="text-sm text-zinc-500 mt-2">Chats from Telegram, Gmail and scheduled tasks show up in the sidebar too.</p>
        {/if}
      </div>
    {:else if chat.loading}
      <div class="p-8 text-sm text-zinc-500 text-center">Loading…</div>
    {:else}
      <div class="max-w-3xl mx-auto px-4 py-6 space-y-6">
        {#each groups as g (g.turnId)}
          <section class="space-y-3">
            {#each g.items as it (it.id)}
              <ItemView item={it} highlight={chat.highlightItem === it.id} />
            {/each}
            {#if g.turn?.status === 'running'}
              <div class="flex items-center gap-2 text-xs text-zinc-500">
                <span class="w-1.5 h-1.5 rounded-full bg-violet-400 animate-ping"></span> Working…
              </div>
            {:else if g.turn}
              <div class="flex items-center gap-2 text-[11px] text-zinc-600">
                {#if g.turn.status === 'failed'}<span class="text-red-400 selectable">Failed: {g.turn.error}</span>{/if}
                {#if g.turn.status === 'interrupted'}<span class="text-amber-400">Stopped</span>{/if}
                <span>{modelLabel(g.turn)}</span>
                {#if usageLine(g.turn.usage)}<span class="ml-auto" title="Tokens and cost for this turn">{usageLine(g.turn.usage)}</span>{/if}
              </div>
            {/if}
          </section>
        {/each}
      </div>
    {/if}
  </div>

  <Composer />
</div>
