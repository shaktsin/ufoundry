<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
</script>

<div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
  {#each app.toasts as t (t.id)}
    <button
      class="text-left text-sm px-3 py-2 rounded-lg shadow-lg border
             {t.kind === 'error' ? 'bg-red-950 border-red-800 text-red-200' : t.kind === 'warn' ? 'bg-amber-950 border-amber-800 text-amber-200' : 'bg-zinc-800 border-zinc-700 text-zinc-200'}"
      onclick={() => app.dismiss(t.id)}
    >
      {t.text}
    </button>
  {/each}
</div>
