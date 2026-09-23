<script lang="ts">
  import { MessageSquare, ShieldCheck, Clock, Puzzle, BarChart3, Settings, SquarePen, FolderTree, FolderOpen } from '@lucide/svelte';
  import { app, type View } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import ProjectSwitcher from './ProjectSwitcher.svelte';
  import ThreadList from './ThreadList.svelte';
  import FileTree from './FileTree.svelte';

  let tab = $state<'chats' | 'files'>('chats');

  const nav: { id: View; label: string; icon: typeof MessageSquare }[] = [
    { id: 'chat', label: 'Chat', icon: MessageSquare },
    { id: 'approvals', label: 'Approvals', icon: ShieldCheck },
    { id: 'tasks', label: 'Tasks', icon: Clock },
    { id: 'extensions', label: 'Skills & MCP', icon: Puzzle },
    { id: 'usage', label: 'Usage', icon: BarChart3 },
    { id: 'settings', label: 'Settings', icon: Settings },
  ];

  const dot = $derived(
    app.conn === 'open' ? 'bg-sage' : app.conn === 'connecting' ? 'bg-amber-warm animate-pulse' : 'bg-rust',
  );
</script>

<aside class="flex flex-col w-64 shrink-0 bg-surface border-r border-line">
  <div class="flex items-center gap-2 px-3 h-12 border-b border-line">
    <ProjectSwitcher />
    <button class="btn-ghost btn-sm shrink-0" title="New chat (⌘N)" aria-label="New chat" onclick={() => chat.newChat()}>
      <SquarePen class="w-4 h-4" />
    </button>
  </div>

  <nav class="px-2 py-2 space-y-0.5 border-b border-line">
    {#each nav as item}
      {@const active = app.view === item.id}
      <button
        class="w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-md text-sm transition-colors
               {active ? 'bg-raised text-ink' : 'text-muted hover:text-ink hover:bg-raised/60'}"
        onclick={() => (app.view = item.id)}
      >
        <item.icon class="w-4 h-4 shrink-0" />
        {item.label}
        {#if item.id === 'approvals' && app.approvals.length > 0}
          <span class="ml-auto min-w-5 h-5 px-1 rounded-full bg-clay-soft/50 text-amber-warm text-[10px] font-bold flex items-center justify-center">
            {app.approvals.length}
          </span>
        {/if}
      </button>
    {/each}
  </nav>

  {#if projects.active}
    <div class="flex px-2 pt-2 gap-1 text-xs">
      <button
        class="flex-1 flex items-center justify-center gap-1.5 py-1 rounded-md transition-colors
               {tab === 'chats' ? 'bg-raised text-ink' : 'text-muted hover:text-ink'}"
        onclick={() => (tab = 'chats')}
      >
        <MessageSquare class="w-3.5 h-3.5" />Chats
      </button>
      <button
        class="flex-1 flex items-center justify-center gap-1.5 py-1 rounded-md transition-colors
               {tab === 'files' ? 'bg-raised text-ink' : 'text-muted hover:text-ink'}"
        onclick={() => (tab = 'files')}
      >
        <FolderTree class="w-3.5 h-3.5" />Files
      </button>
    </div>
  {/if}

  {#if tab === 'files' && projects.active}
    <FileTree />
  {:else}
    <ThreadList />
  {/if}

  <div class="px-3 py-2 border-t border-line flex items-center gap-2 text-[11px] text-muted">
    <span class="w-2 h-2 rounded-full {dot}"></span>
    {#if app.conn === 'open'}
      {#if projects.active}
        <button class="truncate hover:text-ink" title={projects.active.root} onclick={() => (app.view = 'project')}>
          <FolderOpen class="w-3 h-3 inline -mt-0.5" />
          {projects.active.vcs?.branch ?? projects.active.name}
          {#if projects.active.vcs && projects.active.vcs.dirty > 0}
            <span class="text-amber-warm">· {projects.active.vcs.dirty} changed</span>
          {/if}
        </button>
      {:else}
        Engine {app.status?.engineVersion ?? ''}
      {/if}
      {#if app.status && app.status.activeTurns > 0}<span class="ml-auto">{app.status.activeTurns} working</span>{/if}
    {:else if app.conn === 'connecting'}
      Connecting…
    {:else}
      Engine offline
    {/if}
  </div>
</aside>
