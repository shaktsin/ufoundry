<script lang="ts">
  import { Pin, Archive, Search, X, MoreHorizontal, CornerDownRight } from '@lucide/svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { dialog } from '$lib/stores/dialog.svelte';
  import { relTime } from '$lib/format';
  import type { Thread } from '$lib/types';

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
    if (await dialog.confirm(`Delete “${t.title || 'Untitled'}”? This cannot be undone.`, { okLabel: 'Delete', danger: true })) {
      void chat.remove(t);
    }
  }
</script>

<svelte:window onclick={() => (menuFor = null)} />

<div class="flex-1 min-h-0 flex flex-col">
  <div class="px-2 pt-2 pb-1">
    <div class="relative">
      <Search class="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-faint" />
      <input
        id="thread-search"
        class="w-full bg-paper border border-line rounded-md pl-7 pr-7 py-1 text-xs placeholder:text-faint focus:outline-none focus:border-clay"
        placeholder="Search chats (⌘K)"
        bind:value={q}
        oninput={onInput}
      />
      {#if q}
        <button class="absolute right-1.5 top-1/2 -translate-y-1/2 text-faint hover:text-ink" aria-label="Clear search" onclick={clear}>
          <X class="w-3.5 h-3.5" />
        </button>
      {/if}
    </div>
  </div>

  <div class="flex-1 overflow-y-auto px-2 pb-2">
    {#if q.trim()}
      {#if chat.hits.length === 0}
        <p class="text-xs text-muted px-2 py-3">No matches.</p>
      {/if}
      {#each chat.hits as h (h.itemId)}
        <button class="w-full text-left px-2 py-1.5 rounded-md hover:bg-raised/60" onclick={() => chat.open(h.threadId, h.itemId)}>
          <div class="text-xs text-ink truncate">{h.title || 'Untitled'}</div>
          <div class="text-[11px] text-muted line-clamp-2">{h.snippet}</div>
        </button>
      {/each}
    {:else}
      <div class="flex items-center px-2 py-1">
        <span class="text-[10px] font-semibold uppercase tracking-widest text-faint">{chat.showArchived ? 'Archived' : 'Chats'}</span>
        <button class="ml-auto text-[10px] text-muted hover:text-ink" onclick={() => chat.toggleArchived()}>
          {chat.showArchived ? 'Show active' : 'Archived'}
        </button>
      </div>
      {#if chat.threads.length === 0}
        <p class="text-xs text-muted px-2 py-2">{chat.showArchived ? 'Nothing archived.' : 'No chats yet.'}</p>
      {/if}
      {#each chat.rootThreads as t (t.id)}
        {@const active = chat.main.id === t.id}
        <div class="group relative rounded-md {active ? 'bg-raised' : 'hover:bg-raised/50'}">
          {#if renaming === t.id}
            <!-- svelte-ignore a11y_autofocus -->
            <input
              class="w-full bg-paper border border-clay rounded-md px-2 py-1 text-xs focus:outline-none"
              bind:value={renameText}
              autofocus
              onkeydown={(e) => {
                if (e.key === 'Enter') commitRename(t);
                if (e.key === 'Escape') renaming = null;
              }}
              onblur={() => commitRename(t)}
            />
          {:else}
            <button class="w-full text-left px-2 py-1.5 pr-7" onclick={() => chat.open(t.id)} ondblclick={() => startRename(t)}>
              <div class="flex items-center gap-1 text-xs {active ? 'text-ink' : 'text-ink-soft'}">
                {#if t.pinned}<Pin class="w-3 h-3 text-clay shrink-0" />{/if}
                <span class="truncate">{t.title || 'New chat'}</span>
              </div>
              <div class="text-[10px] text-faint flex gap-1.5">
                <span>{relTime(t.updatedAt)}</span>
                {#if t.channel && t.channel !== 'app' && t.channel !== 'cli'}<span>· {t.channel}</span>{/if}
              </div>
            </button>
            <button
              class="absolute right-1 top-1.5 p-0.5 rounded text-faint hover:text-ink hover:bg-line opacity-0 group-hover:opacity-100 {menuFor === t.id ? 'opacity-100' : ''}"
              aria-label="Chat actions"
              onclick={(e) => {
                e.stopPropagation();
                menuFor = menuFor === t.id ? null : t.id;
              }}
            >
              <MoreHorizontal class="w-3.5 h-3.5" />
            </button>
            {#if menuFor === t.id}
              <div
                class="absolute right-1 top-7 z-20 w-36 card py-1 shadow-xl text-xs"
                role="menu"
                tabindex="-1"
                onclick={(e) => e.stopPropagation()}
                onkeydown={() => {}}
              >
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
        <!-- Side chats hang under the chat they came from. -->
        {#each chat.sideChatsOf(t.id) as s (s.id)}
          <button
            class="w-full flex items-center gap-1 pl-5 pr-2 py-1 rounded-md text-[11px] text-muted hover:bg-raised/50 hover:text-ink"
            onclick={() => chat.openSide(s.id)}
          >
            <CornerDownRight class="w-3 h-3 shrink-0 text-faint" />
            <span class="truncate">{s.title || 'Side chat'}</span>
          </button>
        {/each}
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
    color: var(--color-ink-soft);
  }
  .menu-item:hover {
    background: var(--color-raised);
  }
</style>
