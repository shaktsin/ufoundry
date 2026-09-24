<script lang="ts">
  import { tick } from 'svelte';
  import { Sparkles, KeyRound, FolderPlus, Undo2 } from '@lucide/svelte';
  import { app } from '$lib/stores/app.svelte';
  import { chat, type ThreadView } from '$lib/stores/chat.svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import { inspector } from '$lib/stores/inspector.svelte';
  import { dialog } from '$lib/stores/dialog.svelte';
  import { usageLine, fmtUsd } from '$lib/format';
  import ItemView from './ItemView.svelte';
  import Composer from './Composer.svelte';
  import type { Item, Turn } from '$lib/types';

  let { view, variant = 'main' }: { view: ThreadView; variant?: 'main' | 'side' } = $props();

  let scroller: HTMLDivElement | undefined = $state();
  let stick = true;

  interface Group {
    turnId: string;
    turn?: Turn;
    items: Item[];
  }

  const groups = $derived.by(() => {
    const out: Group[] = [];
    const byId = new Map<string, Group>();
    for (const it of view.items) {
      let g = byId.get(it.turnId);
      if (!g) {
        g = { turnId: it.turnId, turn: view.turns.find((t) => t.id === it.turnId), items: [] };
        byId.set(it.turnId, g);
        out.push(g);
      }
      g.items.push(it);
    }
    for (const t of view.turns) if (!byId.has(t.id)) out.push({ turnId: t.id, turn: t, items: [] });
    return out;
  });

  const hasKeys = $derived(app.credentials.some((c) => c.enabled));
  const threadProject = $derived(projects.list.find((p) => p.id === view.thread?.projectId));

  function onScroll() {
    if (!scroller) return;
    stick = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 80;
  }

  $effect(() => {
    void view.items.length;
    void view.items.at(-1)?.text;
    void view.turns.length;
    if (stick) tick().then(() => scroller && (scroller.scrollTop = scroller.scrollHeight));
  });

  $effect(() => {
    const id = view.highlightItem;
    if (!id || view.loading) return;
    tick().then(() => {
      document.getElementById(`item-${id}`)?.scrollIntoView({ block: 'center' });
      stick = false;
      setTimeout(() => (view.highlightItem = null), 2500);
    });
  });

  $effect(() => {
    void view.id;
    stick = true;
  });

  function modelLabel(t: Turn): string {
    const r = t.resolved || {};
    const m = app.models.find((x) => x.provider === r.provider && x.id === r.model);
    const name = m?.displayName || r.model || '';
    const cx = r.complexity ? `${r.complexity}${t.autoPicked ? ' (auto)' : ''}` : '';
    return [name, cx].filter(Boolean).join(' · ');
  }

  /**
   * When a turn moved off its first model — a rate limit, an outage — the
   * footer says so, so a slower or cheaper answer is never a mystery.
   */
  function switchNote(t: Turn): string {
    const trail = t.routeTrail ?? [];
    if (trail.length < 2) return '';
    const names = trail.map((s) => s.displayName || s.model);
    const used = trail[trail.length - 1];
    const first = trail[0];
    if (first.model === used.model) return `switched key after ${statusWord(first.status)}`;
    return `switched from ${names[0]} after ${statusWord(first.status)}`;
  }

  function statusWord(status: string): string {
    return (
      {
        rate_limited: 'a rate limit',
        server_error: 'a provider error',
        unauthorized: 'a key being refused',
        timeout: 'a timeout',
        context_too_long: 'a prompt too long for it',
        unknown_model: 'it being unavailable',
      }[status] ?? 'an error'
    );
  }

  /** Files a turn changed, for the "undo this turn" control. */
  function changedFiles(g: Group): string[] {
    return g.items.filter((i) => i.kind === 'fileChange').map((i) => (i.data as { path?: string })?.path ?? '');
  }

  async function undoTurn(g: Group) {
    const files = changedFiles(g).filter(Boolean);
    const ok = await dialog.confirm(
      `Put ${files.length === 1 ? files[0] : `${files.length} files`} back the way this turn found it?`,
      { okLabel: 'Undo changes' },
    );
    if (ok) await inspector.revertTurn(g.turnId);
  }

  async function openProject() {
    const path = await dialog.prompt('Which folder should the agent work in?', { okLabel: 'Open project' });
    if (path?.trim()) await projects.add(path.trim());
  }
</script>

<div class="flex-1 min-h-0 flex flex-col">
  {#if variant === 'main'}
    <header class="min-h-14 shrink-0 border-b border-line flex items-center px-5 gap-3 bg-paper/90">
      <div class="min-w-0">
        <h1 class="text-sm font-semibold truncate tracking-[-0.01em]">{view.title}</h1>
        <div class="flex items-center gap-2 mt-0.5 text-[10px] text-muted min-w-0">
          {#if threadProject}
            <span class="font-medium text-ink-soft shrink-0">{threadProject.name}</span>
            <span class="font-mono truncate" title={threadProject.root}>{threadProject.root}</span>
            {#if threadProject.vcs?.branch}<span class="shrink-0">branch {threadProject.vcs.branch}</span>{/if}
            <span class="shrink-0">{threadProject.tools.shell === false ? 'shell off' : 'shell on'}</span>
            <span class="shrink-0">{threadProject.tools.network ? 'network on' : 'network off'}</span>
          {:else}
            <span>No project</span><span>read-only workspace</span>
          {/if}
        </div>
      </div>
      {#if view.thread?.usage?.requests}
        <span class="ml-auto text-[11px] text-muted" title="Tokens and cost for this chat">
          {fmtUsd(view.thread.usage.costUsd)} / {view.thread.usage.requests} requests
        </span>
      {/if}
    </header>
  {/if}

  <div class="flex-1 min-h-0 overflow-y-auto" bind:this={scroller} onscroll={onScroll}>
    {#if view.context}
      <div class="mx-4 mt-3 px-3 py-2 rounded-lg bg-raised/60 border border-line text-xs text-muted whitespace-pre-wrap selectable">
        {view.context}
      </div>
    {/if}

    {#if !view.id && variant === 'main'}
      <div class="h-full flex flex-col items-center justify-center text-center px-6">
        <Sparkles class="w-8 h-8 text-clay mb-3" />
        <h2 class="text-lg font-semibold">
          {projects.active ? `What should we do in ${projects.active.name}?` : 'What can I help with?'}
        </h2>
        {#if app.conn === 'open' && !hasKeys}
          <p class="text-sm text-muted mt-2 max-w-sm">Add an API key for Claude, OpenAI, Gemini or a local model server to get started.</p>
          <button class="btn-primary mt-4" onclick={() => (app.view = 'settings')}><KeyRound class="w-4 h-4" />Add an API key</button>
        {:else if !projects.active}
          <p class="text-sm text-muted mt-2 max-w-sm">
            Without a project I can read and answer, but not change files. Open a folder to let me work in it.
          </p>
          <button class="btn-primary mt-4" onclick={openProject}><FolderPlus class="w-4 h-4" />Open a folder</button>
        {:else}
          <p class="text-sm text-muted mt-2 max-w-md">
            I work inside <span class="font-mono text-xs">{projects.active.root}</span> and nowhere else. Ask for a change, and every
            edit shows up here with its diff.
          </p>
        {/if}
      </div>
    {:else if view.loading}
      <div class="p-8 text-sm text-muted text-center">Loading…</div>
    {:else}
      <div class="{variant === 'main' ? 'max-w-3xl' : ''} mx-auto px-4 py-6 space-y-6">
        {#each groups as g (g.turnId)}
          <section class="space-y-3 group/turn">
            {#each g.items as it (it.id)}
              <ItemView item={it} highlight={view.highlightItem === it.id} {view} />
            {/each}
            {#if g.turn?.status === 'running'}
              <div class="flex items-center gap-2 text-xs text-muted">
                <span class="w-1.5 h-1.5 rounded-full bg-clay animate-ping"></span> Working…
              </div>
            {:else if g.turn}
              <div class="flex items-center gap-2 text-[11px] text-faint">
                {#if g.turn.status === 'failed'}<span class="text-rust selectable">Failed: {g.turn.error}</span>{/if}
                {#if g.turn.status === 'interrupted'}<span class="text-amber-warm">Stopped</span>{/if}
                <span>{modelLabel(g.turn)}</span>
                {#if switchNote(g.turn)}
                  <span class="text-amber-warm" title={(g.turn.routeTrail ?? []).map((s) => `${s.displayName || s.model} (${s.credentialLabel}): ${s.status}${s.error ? ` — ${s.error}` : ''}`).join('\n')}>
                    {switchNote(g.turn)}
                  </span>
                {/if}
                {#if changedFiles(g).length > 0}
                  <button
                    class="opacity-0 group-hover/turn:opacity-100 transition-opacity hover:text-ink flex items-center gap-1"
                    onclick={() => undoTurn(g)}
                  >
                    <Undo2 class="w-3 h-3" />undo this turn
                  </button>
                {/if}
                {#if usageLine(g.turn.usage)}
                  <span class="ml-auto" title="Tokens and cost for this turn">{usageLine(g.turn.usage)}</span>
                {/if}
              </div>
            {/if}
          </section>
        {/each}
      </div>
    {/if}
  </div>

  <Composer {view} compact={variant === 'side'} />
</div>
