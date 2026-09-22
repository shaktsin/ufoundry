<script lang="ts">
  import { MessageSquare, ShieldCheck, Clock, Puzzle, BarChart3, Settings, SquarePen } from '@lucide/svelte';
  import { app, type View } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import ThreadList from './ThreadList.svelte';

  const nav: { id: View; label: string; icon: typeof MessageSquare }[] = [
    { id: 'chat', label: 'Chat', icon: MessageSquare },
    { id: 'approvals', label: 'Approvals', icon: ShieldCheck },
    { id: 'tasks', label: 'Tasks', icon: Clock },
    { id: 'extensions', label: 'Skills & MCP', icon: Puzzle },
    { id: 'usage', label: 'Usage', icon: BarChart3 },
    { id: 'settings', label: 'Settings', icon: Settings },
  ];

  const dot = $derived(
    app.conn === 'open' ? 'bg-emerald-500' : app.conn === 'connecting' ? 'bg-amber-500 animate-pulse' : 'bg-red-500',
  );
</script>

<aside class="flex flex-col w-64 shrink-0 bg-zinc-900/70 border-r border-zinc-800">
  <div class="flex items-center gap-2 px-3 h-12 border-b border-zinc-800">
    <div class="w-6 h-6 rounded-md bg-violet-600 flex items-center justify-center shrink-0 text-[11px] font-bold text-white">U</div>
    <span class="text-sm font-semibold">UFoundry</span>
    <button class="ml-auto btn-ghost btn-sm" title="New chat (⌘N)" aria-label="New chat" onclick={() => chat.newChat()}>
      <SquarePen class="w-4 h-4" />
    </button>
  </div>

  <nav class="px-2 py-2 space-y-0.5 border-b border-zinc-800">
    {#each nav as item}
      {@const active = app.view === item.id}
      <button
        class="w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-md text-sm transition-colors
               {active ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/60'}"
        onclick={() => (app.view = item.id)}
      >
        <item.icon class="w-4 h-4 shrink-0" />
        {item.label}
        {#if item.id === 'approvals' && app.approvals.length > 0}
          <span class="ml-auto min-w-5 h-5 px-1 rounded-full bg-amber-500/20 text-amber-400 text-[10px] font-bold flex items-center justify-center">
            {app.approvals.length}
          </span>
        {/if}
      </button>
    {/each}
  </nav>

  <ThreadList />

  <div class="px-3 py-2 border-t border-zinc-800 flex items-center gap-2 text-[11px] text-zinc-500">
    <span class="w-2 h-2 rounded-full {dot}"></span>
    {#if app.conn === 'open'}
      Engine {app.status?.engineVersion ?? ''}
      {#if app.status && app.status.activeTurns > 0}<span class="ml-auto">{app.status.activeTurns} running</span>{/if}
    {:else if app.conn === 'connecting'}
      Connecting…
    {:else}
      Engine offline
    {/if}
  </div>
</aside>
