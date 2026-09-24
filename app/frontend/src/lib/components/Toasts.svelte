<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
</script>

<div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
  {#each app.toasts as t (t.id)}
    <button
      class="text-left text-sm px-3 py-2 rounded-lg shadow-lg border
             {t.kind === 'error' ? 'bg-rust-soft border-rust/40 text-rust' : t.kind === 'warn' ? 'bg-clay-soft/40 border-clay/40 text-amber-warm' : 'bg-raised border-line-strong text-ink'}"
      onclick={() => app.dismiss(t.id)}
    >
      {t.text}
    </button>
  {/each}
</div>
