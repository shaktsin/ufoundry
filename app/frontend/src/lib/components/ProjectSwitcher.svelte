<script lang="ts">
  import { ChevronsUpDown, Plus, Check, FolderGit2, AlertTriangle } from '@lucide/svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { createProject } from '$lib/createProject';

  let open = $state(false);

  async function choose(id: string | null) {
    open = false;
    await projects.open(id);
    chat.newChat();
    await chat.loadThreads();
  }

  async function add() {
    open = false;
    const p = await createProject();
    if (p) {
      chat.newChat();
      await chat.loadThreads();
    }
  }
</script>

<svelte:window onclick={() => (open = false)} />

<div class="relative flex-1 min-w-0">
  <button
    class="w-full flex items-center gap-1.5 px-1.5 py-1 rounded-md hover:bg-raised text-left"
    onclick={(e) => {
      e.stopPropagation();
      open = !open;
    }}
  >
    <div class="w-5 h-5 rounded bg-clay text-white text-[10px] font-bold flex items-center justify-center shrink-0">
      {(projects.active?.name ?? 'U').slice(0, 1).toUpperCase()}
    </div>
    <span class="text-sm font-medium truncate">{projects.active?.name ?? 'No project'}</span>
    {#if projects.active?.missing}<AlertTriangle class="w-3.5 h-3.5 text-rust shrink-0" />{/if}
    <ChevronsUpDown class="w-3.5 h-3.5 text-faint ml-auto shrink-0" />
  </button>

  {#if open}
    <div
      class="absolute left-0 top-9 z-30 w-72 card py-1 shadow-xl text-sm"
      role="menu"
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
      onkeydown={() => {}}
    >
      <p class="px-3 py-1 text-[10px] uppercase tracking-widest text-faint">Projects</p>
      {#each projects.list as p (p.id)}
        <button class="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-raised text-left" onclick={() => choose(p.id)}>
          <FolderGit2 class="w-3.5 h-3.5 text-muted shrink-0" />
          <span class="min-w-0 flex-1">
            <span class="block truncate">{p.name}</span>
            <span class="block text-[11px] text-faint truncate">{p.root}</span>
          </span>
          {#if p.missing}<AlertTriangle class="w-3.5 h-3.5 text-rust" />{/if}
          {#if projects.activeId === p.id}<Check class="w-3.5 h-3.5 text-clay" />{/if}
        </button>
      {/each}
      {#if projects.list.length === 0}
        <p class="px-3 py-2 text-xs text-muted">No projects yet. Open a folder to let the agent work in it.</p>
      {/if}
      <div class="border-t border-line my-1"></div>
      <button class="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-raised text-left" onclick={add}>
        <Plus class="w-3.5 h-3.5" /> New project…
      </button>
      <button
        class="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-raised text-left text-muted"
        onclick={() => choose(null)}
      >
        Chat without a project
      </button>
      {#if projects.active}
        <button
          class="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-raised text-left"
          onclick={() => {
            open = false;
            app.view = 'project';
          }}
        >
          Project settings…
        </button>
      {/if}
    </div>
  {/if}
</div>
