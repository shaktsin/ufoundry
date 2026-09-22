<script lang="ts">
  import { Pin, Archive, Search, X, MoreHorizontal } from '@lucide/svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { relTime } from '$lib/format';
  import type { Thread } from '$lib/types';
  import { dialog } from '$lib/stores/dialog.svelte';

  let q = $state('');
  let timer: ReturnType<typeof setTimeout> | undefined;
  let menuFor = $state<string | null>(null);
  let renaming = $state<string | null>(null);
  let renameText = $state('');

  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(() => chat.search(q), 200);
  }

  function clear() {
    q = '';
    chat.search('');
  }

  function openHit(threadId: string, itemId: string) {
    chat.highlightItem = itemId;
    void chat.open(threadId);
  }

  function startRename(t: Thread) {
    menuFor = null;
    renaming = t.id;
    renameText = t.title;
  }

  async function commitRename(t: Thread) {
    const title = renameText.trim();
    renaming = null;
    if (title && title !== t.title) await chat.rename(t.id, title);
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
    if (await dialog.confirm(`Delete “${t.title || 'Untitled'}”? This cannot be undone.`, { okLabel: 'Delete', danger: true })) void chat.remove(t);
  }
</script>

<svelte:window onclick={() => (menuFor = null)} />

<div class="flex-1 min-h-0 flex flex-col">
  <div class="px-2 pt-2 pb-1">
    <div class="relative">
      <Search class="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-zinc-500" />
      <input
        id="thread-search"
        class="w-full bg-zinc-800/70 border border-zinc-800 rounded-md pl-7 pr-7 py-1 text-xs placeholder:text-zinc-500 focus:outline-none focus:border-violet-500"
        placeholder="Search chats (⌘K)"
        bind:value={q}
        oninput={onInput}
      />
      {#if q}
        <button class="absolute right-1.5 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300" aria-label="Clear search" onclick={clear}>
          <X class="w-3.5 h-3.5" />
        </button>
      {/if}
    </div>
  </div>

  <div class="flex-1 overflow-y-auto px-2 pb-2">
    {#if q.trim()}
      {#if chat.hits.length === 0}
        <p class="text-xs text-zinc-500 px-2 py-3">No matches.</p>
      {/if}
      {#each chat.hits as h (h.itemId)}
        <button class="w-full text-left px-2 py-1.5 rounded-md hover:bg-zinc-800/60" onclick={() => openHit(h.threadId, h.itemId)}>
          <div class="text-xs text-zinc-200 truncate">{h.title || 'Untitled'}</div>
          <div class="text-[11px] text-zinc-500 line-clamp-2">{h.snippet}</div>
        </button>
      {/each}
    {:else}
      <div class="flex items-center px-2 py-1">
        <span class="text-[10px] font-semibold uppercase tracking-widest text-zinc-600">{chat.showArchived ? 'Archived' : 'Chats'}</span>
        <button class="ml-auto text-[10px] text-zinc-500 hover:text-zinc-300" onclick={() => chat.toggleArchived()}>
          {chat.showArchived ? 'Show active' : 'Archived'}
        </button>
      </div>
      {#if chat.threads.length === 0}
        <p class="text-xs text-zinc-500 px-2 py-2">{chat.showArchived ? 'Nothing archived.' : 'No chats yet.'}</p>
      {/if}
      {#each chat.threads as t (t.id)}
        {@const active = chat.activeId === t.id}
        <div class="group relative rounded-md {active ? 'bg-zinc-800' : 'hover:bg-zinc-800/50'}">
          {#if renaming === t.id}
            <!-- svelte-ignore a11y_autofocus -->
            <input
              class="w-full bg-zinc-800 border border-violet-500 rounded-md px-2 py-1 text-xs focus:outline-none"
              bind:value={renameText}
              autofocus
              onkeydown={(e) => { if (e.key === 'Enter') commitRename(t); if (e.key === 'Escape') renaming = null; }}
              onblur={() => commitRename(t)}
            />
          {:else}
            <button class="w-full text-left px-2 py-1.5 pr-7" onclick={() => chat.open(t.id)} ondblclick={() => startRename(t)}>
              <div class="flex items-center gap-1 text-xs {active ? 'text-zinc-100' : 'text-zinc-300'}">
                {#if t.pinned}<Pin class="w-3 h-3 text-violet-400 shrink-0" />{/if}
                <span class="truncate">{t.title || 'New chat'}</span>
              </div>
              <div class="text-[10px] text-zinc-500 flex gap-1.5">
                <span>{relTime(t.updatedAt)}</span>
                {#if t.channel && t.channel !== 'app' && t.channel !== 'cli'}<span>· {t.channel}</span>{/if}
              </div>
            </button>
            <button
              class="absolute right-1 top-1.5 p-0.5 rounded text-zinc-500 hover:text-zinc-200 hover:bg-zinc-700 opacity-0 group-hover:opacity-100 {menuFor === t.id ? 'opacity-100' : ''}"
              aria-label="Chat actions"
              onclick={(e) => { e.stopPropagation(); menuFor = menuFor === t.id ? null : t.id; }}
            >
              <MoreHorizontal class="w-3.5 h-3.5" />
            </button>
            {#if menuFor === t.id}
              <div class="absolute right-1 top-7 z-20 w-36 card py-1 shadow-xl text-xs" role="menu" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
                <button class="menu-item" onclick={() => { menuFor = null; chat.pin(t); }}><Pin class="w-3.5 h-3.5" />{t.pinned ? 'Unpin' : 'Pin'}</button>
                <button class="menu-item" onclick={() => startRename(t)}>Rename</button>
                <button class="menu-item" onclick={() => { menuFor = null; chat.fork(t); }}>Fork</button>
                <button class="menu-item" onclick={() => doExport(t)}>Copy as Markdown</button>
                <button class="menu-item" onclick={() => { menuFor = null; chat.archive(t); }}><Archive class="w-3.5 h-3.5" />{t.archived ? 'Unarchive' : 'Archive'}</button>
                <button class="menu-item text-red-400" onclick={() => confirmDelete(t)}>Delete</button>
              </div>
            {/if}
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .menu-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.35rem 0.75rem;
    text-align: left;
    color: rgb(212 212 216);
  }
  .menu-item:hover {
    background: rgb(39 39 42);
  }
</style>
