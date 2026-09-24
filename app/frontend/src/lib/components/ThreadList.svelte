<script lang="ts">
  import {
    Archive, ChevronDown, ChevronRight, CornerDownRight, Folder, MoreHorizontal, Pencil, Pin, Plus, Search, X,
  } from '@lucide/svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import { dialog } from '$lib/stores/dialog.svelte';
  import { relTime } from '$lib/format';
  import type { Project, Thread } from '$lib/types';

  let q = $state('');
  let timer: ReturnType<typeof setTimeout> | undefined;
  let menuFor = $state<string | null>(null);
  let renaming = $state<string | null>(null);
  let renameText = $state('');
  let renamingProject = $state<string | null>(null);
  let projectName = $state('');
  let expanded = $state<Record<string, boolean>>({});

  const groups = $derived(projects.list.map((project) => ({
    project,
    threads: chat.rootThreads.filter((thread) => thread.projectId === project.id),
  })));
  const looseThreads = $derived(chat.rootThreads.filter((thread) => !thread.projectId));

  function isExpanded(id: string) { return expanded[id] ?? true; }
  function toggle(id: string) { expanded = { ...expanded, [id]: !isExpanded(id) }; }
  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(() => chat.search(q), 200);
  }
  function clear() { q = ''; chat.search(''); }
  function startRename(t: Thread) { menuFor = null; renaming = t.id; renameText = t.title; }
  async function commitRename(t: Thread) {
    const title = renameText.trim();
    renaming = null;
    if (title && title !== t.title) await chat.rename(t.id, title);
  }
  function startProjectRename(project: Project) {
    renamingProject = project.id;
    projectName = project.name;
  }
  async function commitProjectRename(project: Project) {
    const name = projectName.trim();
    renamingProject = null;
    if (name && name !== project.name) await projects.rename(project.id, name);
  }
  async function openProject(project: Project) {
    await projects.open(project.id);
    app.view = 'project';
  }
  async function doExport(t: Thread) {
    menuFor = null;
    const md = await chat.exportMarkdown(t);
    if (!md) return;
    try {
      await navigator.clipboard.writeText(md);
      app.toast('info', 'Copied the conversation as Markdown.');
    } catch {
      app.toast('error', 'Could not copy to the clipboard.');
    }
  }
  async function confirmDelete(t: Thread) {
    menuFor = null;
    if (await dialog.confirm(`Delete “${t.title || 'Untitled'}”? This cannot be undone.`, { okLabel: 'Delete', danger: true })) {
      void chat.remove(t);
    }
  }
</script>

<svelte:window onclick={() => (menuFor = null)} />

{#snippet threadRow(t: Thread)}
  {@const active = chat.main.id === t.id}
  <div class="group/thread relative rounded-lg {active ? 'bg-accent-soft' : 'hover:bg-raised/70'}">
    {#if renaming === t.id}
      <!-- svelte-ignore a11y_autofocus -->
      <input
        class="w-full bg-paper border border-accent rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
        bind:value={renameText}
        autofocus
        onkeydown={(e) => {
          if (e.key === 'Enter') commitRename(t);
          if (e.key === 'Escape') renaming = null;
        }}
        onblur={() => commitRename(t)}
      />
    {:else}
      <button class="w-full text-left px-2.5 py-1.5 pr-8" onclick={() => chat.open(t.id)} ondblclick={() => startRename(t)}>
        <div class="flex items-center gap-1.5 text-xs {active ? 'text-accent-strong font-medium' : 'text-ink-soft'}">
          {#if t.pinned}<Pin class="w-3 h-3 text-accent shrink-0" />{/if}
          <span class="truncate">{t.title || 'New chat'}</span>
        </div>
        <div class="text-[10px] text-faint flex gap-1.5 mt-0.5">
          <span>{relTime(t.updatedAt)}</span>
          {#if t.channel && t.channel !== 'app' && t.channel !== 'cli'}<span>{t.channel}</span>{/if}
        </div>
      </button>
      <button
        class="absolute right-1.5 top-1.5 p-1 rounded-md text-faint hover:text-ink hover:bg-line opacity-0 group-hover/thread:opacity-100 {menuFor === t.id ? 'opacity-100' : ''}"
        aria-label="Chat actions"
        onclick={(e) => { e.stopPropagation(); menuFor = menuFor === t.id ? null : t.id; }}
      ><MoreHorizontal class="w-3.5 h-3.5" /></button>
      {#if menuFor === t.id}
        <div class="absolute right-1 top-8 z-20 w-40 popover py-1 text-xs" role="menu" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
          <button class="menu-item" onclick={() => { menuFor = null; chat.pin(t); }}><Pin class="w-3.5 h-3.5" />{t.pinned ? 'Unpin' : 'Pin'}</button>
          <button class="menu-item" onclick={() => startRename(t)}>Rename</button>
          <button class="menu-item" onclick={() => { menuFor = null; chat.fork(t); }}>Fork</button>
          <button class="menu-item" onclick={() => doExport(t)}>Copy as Markdown</button>
          <button class="menu-item" onclick={() => { menuFor = null; chat.archive(t); }}><Archive class="w-3.5 h-3.5" />{t.archived ? 'Unarchive' : 'Archive'}</button>
          <button class="menu-item text-rust" onclick={() => confirmDelete(t)}>Delete</button>
        </div>
      {/if}
    {/if}
  </div>
  {#each chat.sideChatsOf(t.id) as side (side.id)}
    <button class="w-full flex items-center gap-1.5 pl-6 pr-2 py-1 rounded-md text-[11px] text-muted hover:bg-raised/60 hover:text-ink" onclick={() => chat.openSide(side.id)}>
      <CornerDownRight class="w-3 h-3 shrink-0 text-faint" />
      <span class="truncate">{side.title || 'Side chat'}</span>
    </button>
  {/each}
{/snippet}

<div class="flex-1 min-h-0 flex flex-col">
  <div class="px-3 pt-3 pb-2">
    <div class="relative">
      <Search class="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" />
      <input id="thread-search" class="rail-search" placeholder="Search chats" bind:value={q} oninput={onInput} />
      {#if q}
        <button class="absolute right-2 top-1/2 -translate-y-1/2 text-faint hover:text-ink" aria-label="Clear search" onclick={clear}><X class="w-3.5 h-3.5" /></button>
      {/if}
    </div>
  </div>

  <div class="flex items-center px-4 pb-1.5">
    <span class="section-label">Chats</span>
    <button class="ml-auto text-[10px] text-muted hover:text-ink" onclick={() => chat.toggleArchived()}>{chat.showArchived ? 'Active' : 'Archived'}</button>
  </div>

  <div class="flex-1 overflow-y-auto px-2.5 pb-3 space-y-1">
    {#if q.trim()}
      {#if chat.hits.length === 0}<p class="text-xs text-muted px-2 py-3">No matches.</p>{/if}
      {#each chat.hits as hit (hit.itemId)}
        <button class="w-full text-left px-2.5 py-2 rounded-lg hover:bg-raised/70" onclick={() => chat.open(hit.threadId, hit.itemId)}>
          <div class="text-xs text-ink truncate">{hit.title || 'Untitled'}</div>
          <div class="text-[11px] text-muted line-clamp-2 mt-0.5">{hit.snippet}</div>
        </button>
      {/each}
    {:else}
      {#each groups as group (group.project.id)}
        <section class="pb-1">
          <div class="group/project flex items-center gap-1 rounded-lg hover:bg-raised/50">
            <button class="p-1.5 text-faint hover:text-ink" aria-label={`Toggle ${group.project.name}`} onclick={() => toggle(group.project.id)}>
              {#if isExpanded(group.project.id)}<ChevronDown class="w-3.5 h-3.5" />{:else}<ChevronRight class="w-3.5 h-3.5" />{/if}
            </button>
            <Folder class="w-3.5 h-3.5 text-muted shrink-0" />
            {#if renamingProject === group.project.id}
              <!-- svelte-ignore a11y_autofocus -->
              <input
                class="min-w-0 flex-1 bg-paper border border-accent rounded px-1.5 py-0.5 text-xs focus:outline-none"
                bind:value={projectName}
                autofocus
                onkeydown={(e) => {
                  if (e.key === 'Enter') commitProjectRename(group.project);
                  if (e.key === 'Escape') renamingProject = null;
                }}
                onblur={() => commitProjectRename(group.project)}
              />
            {:else}
              <button class="min-w-0 flex-1 text-left text-[11px] font-semibold text-ink-soft truncate py-1.5" title={group.project.root} onclick={() => openProject(group.project)} ondblclick={() => startProjectRename(group.project)}>{group.project.name}</button>
            {/if}
            <span class="text-[10px] text-faint">{group.threads.length}</span>
            <button class="p-1.5 text-faint hover:text-accent opacity-0 group-hover/project:opacity-100" aria-label={`Rename ${group.project.name}`} title="Rename project" onclick={() => startProjectRename(group.project)}><Pencil class="w-3 h-3" /></button>
            <button class="p-1.5 text-faint hover:text-accent opacity-0 group-hover/project:opacity-100" aria-label={`New chat in ${group.project.name}`} title={`New chat in ${group.project.name}`} onclick={() => chat.newChatFor(group.project.id)}><Plus class="w-3.5 h-3.5" /></button>
          </div>
          {#if isExpanded(group.project.id)}
            <div class="ml-3 pl-2 border-l border-line space-y-0.5">
              {#if group.threads.length === 0}<p class="text-[11px] text-faint px-2 py-1">No chats yet</p>{/if}
              {#each group.threads as thread (thread.id)}{@render threadRow(thread)}{/each}
            </div>
          {/if}
        </section>
      {/each}

      <section class="pb-1">
        <div class="group/project flex items-center gap-1 rounded-lg hover:bg-raised/50">
          <button class="p-1.5 text-faint hover:text-ink" aria-label="Toggle chats without a project" onclick={() => toggle('_loose')}>
            {#if isExpanded('_loose')}<ChevronDown class="w-3.5 h-3.5" />{:else}<ChevronRight class="w-3.5 h-3.5" />{/if}
          </button>
          <div class="min-w-0 flex-1 text-[11px] font-semibold text-ink-soft py-1.5">Without a project</div>
          <span class="text-[10px] text-faint">{looseThreads.length}</span>
          <button class="p-1.5 text-faint hover:text-accent opacity-0 group-hover/project:opacity-100" aria-label="New chat without a project" title="New chat without a project" onclick={() => chat.newChatFor(null)}><Plus class="w-3.5 h-3.5" /></button>
        </div>
        {#if isExpanded('_loose')}
          <div class="ml-3 pl-2 border-l border-line space-y-0.5">
            {#if looseThreads.length === 0}<p class="text-[11px] text-faint px-2 py-1">No chats yet</p>{/if}
            {#each looseThreads as thread (thread.id)}{@render threadRow(thread)}{/each}
          </div>
        {/if}
      </section>
    {/if}
  </div>
</div>

<style>
  .menu-item { display: flex; align-items: center; gap: 0.5rem; width: 100%; padding: 0.4rem 0.75rem; text-align: left; color: var(--color-ink-soft); }
  .menu-item:hover { background: var(--color-raised); }
</style>
