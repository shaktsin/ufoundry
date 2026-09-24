<script lang="ts">
  import { projects } from '$lib/stores/projects.svelte';
  import FileNode from './FileNode.svelte';

  const entries = $derived(projects.tree[''] ?? []);
</script>

<div class="flex-1 min-h-0 overflow-y-auto px-1 py-2">
  {#if !projects.active}
    <p class="text-xs text-muted px-2 py-2">Open a project to browse its files.</p>
  {:else if entries.length === 0}
    <p class="text-xs text-muted px-2 py-2">This folder is empty.</p>
  {/if}
  <FileNode path="" depth={0} />
</div>
