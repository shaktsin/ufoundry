<script lang="ts">
  import {
    BarChart3,
    Blocks,
    FolderKanban,
    MessageCircle,
    PanelLeftClose,
    PanelLeftOpen,
    Repeat2,
    Settings,
    ShieldCheck,
    SquarePen,
  } from '@lucide/svelte';
  import { app, type View } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import ThreadList from './ThreadList.svelte';

  const KEY = 'ufoundry.railCollapsed';
  let collapsed = $state(false);
  try { collapsed = localStorage.getItem(KEY) === '1'; } catch { /* use default */ }

  const nav: { id: View; label: string; icon: typeof MessageCircle }[] = [
    { id: 'chat', label: 'Chat', icon: MessageCircle },
    { id: 'project', label: 'Project', icon: FolderKanban },
    { id: 'extensions', label: 'Plugin', icon: Blocks },
    { id: 'tasks', label: 'Periodic', icon: Repeat2 },
  ];

  function toggleRail() {
    collapsed = !collapsed;
    try { localStorage.setItem(KEY, collapsed ? '1' : '0'); } catch { /* storage is optional */ }
  }
</script>

<aside class="app-rail {collapsed ? 'is-collapsed' : ''}">
  <div class="rail-brand">
    <div class="brand-mark">u</div>
    {#if !collapsed}<span class="brand-name">ufoundry</span>{/if}
    <button class="icon-button ml-auto" title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} onclick={toggleRail}>
      {#if collapsed}<PanelLeftOpen class="w-4 h-4" />{:else}<PanelLeftClose class="w-4 h-4" />{/if}
    </button>
  </div>

  <div class="px-2.5 pt-2.5">
    <button class="new-chat-button {collapsed ? 'justify-center px-0' : ''}" title="New chat" onclick={() => chat.newChat()}>
      <SquarePen class="w-4 h-4 shrink-0" />
      {#if !collapsed}<span>New chat</span><kbd>⌘N</kbd>{/if}
    </button>
  </div>

  <nav class="rail-nav" aria-label="Main navigation">
    {#each nav as item}
      {@const active = app.view === item.id}
      <button class="rail-nav-item {active ? 'is-active' : ''} {collapsed ? 'justify-center px-0' : ''}" title={collapsed ? item.label : undefined} onclick={() => (app.view = item.id)}>
        <item.icon class="w-[17px] h-[17px] shrink-0" strokeWidth={1.8} />
        {#if !collapsed}<span>{item.label}</span>{/if}
      </button>
    {/each}
  </nav>

  {#if !collapsed}<ThreadList />{:else}<div class="flex-1"></div>{/if}

  <div class="rail-footer">
    <div class="flex items-center gap-1">
      <button class="footer-button {app.view === 'approvals' ? 'is-active' : ''}" title="Approvals" aria-label="Approvals" onclick={() => (app.view = 'approvals')}>
        <ShieldCheck class="w-4 h-4" strokeWidth={1.8} />
        {#if !collapsed}<span>Approvals</span>{/if}
        {#if app.approvals.length > 0}<span class="count-badge">{app.approvals.length}</span>{/if}
      </button>
      {#if !collapsed}
        <button class="icon-button ml-auto" title="Usage" aria-label="Usage" onclick={() => (app.view = 'usage')}><BarChart3 class="w-4 h-4" strokeWidth={1.8} /></button>
        <button class="icon-button" title="Settings" aria-label="Settings" onclick={() => (app.view = 'settings')}><Settings class="w-4 h-4" strokeWidth={1.8} /></button>
      {/if}
    </div>
    {#if collapsed}
      <button class="footer-button justify-center" title="Usage" aria-label="Usage" onclick={() => (app.view = 'usage')}><BarChart3 class="w-4 h-4" /></button>
      <button class="footer-button justify-center" title="Settings" aria-label="Settings" onclick={() => (app.view = 'settings')}><Settings class="w-4 h-4" /></button>
    {:else}
      <div class="engine-line">
        <span class="engine-indicator" class:is-online={app.conn === 'open'} class:is-connecting={app.conn === 'connecting'}></span>
        <span>{app.conn === 'open' ? 'Engine online' : app.conn === 'connecting' ? 'Connecting' : 'Engine offline'}</span>
        {#if app.status && app.status.activeTurns > 0}<span class="ml-auto">{app.status.activeTurns} running</span>{/if}
      </div>
    {/if}
  </div>
</aside>
