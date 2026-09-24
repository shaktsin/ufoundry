<script lang="ts">
  import { ChevronRight, File, Folder } from '@lucide/svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import { inspector } from '$lib/stores/inspector.svelte';
  import FileNode from './FileNode.svelte';
  import type { FileEntry } from '$lib/types';

  let { path, depth }: { path: string; depth: number } = $props();
  const entries = $derived<FileEntry[]>(projects.tree[path] ?? []);
</script>

{#each entries as e (e.path)}
  {#if e.dir}
    <button
      class="w-full flex items-center gap-1 px-2 py-0.5 rounded hover:bg-raised/60 text-left text-xs text-ink-soft"
      style="padding-left: {depth * 10 + 4}px"
      onclick={() => projects.toggleDir(e.path)}
    >
      <ChevronRight class="w-3 h-3 text-faint transition-transform {projects.expanded[e.path] ? 'rotate-90' : ''}" />
      <Folder class="w-3.5 h-3.5 text-faint shrink-0" />
      <span class="truncate">{e.name}</span>
    </button>
    {#if projects.expanded[e.path]}
      <FileNode path={e.path} depth={depth + 1} />
    {/if}
  {:else}
    <button
      class="w-full flex items-center gap-1 px-2 py-0.5 rounded hover:bg-raised/60 text-left text-xs text-muted"
      style="padding-left: {depth * 10 + 18}px"
      onclick={() => inspector.openFile(e.path)}
    >
      <File class="w-3.5 h-3.5 shrink-0" />
      <span class="truncate">{e.name}</span>
    </button>
  {/if}
{/each}
