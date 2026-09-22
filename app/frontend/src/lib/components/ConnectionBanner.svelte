<script lang="ts">
  import { app } from '$lib/stores/app.svelte';
</script>

{#if app.conn !== 'open'}
  <div class="px-4 py-2 text-xs flex items-center gap-3 border-b {app.conn === 'connecting' ? 'bg-amber-500/10 border-amber-500/20 text-amber-300' : 'bg-red-500/10 border-red-500/20 text-red-300'}">
    {#if app.conn === 'connecting'}
      Connecting to the UFoundry engine…
    {:else}
      <span>
        Can't reach the engine{app.connError ? `: ${app.connError}` : ''}.
        {#if app.shell === 'mac'}The app starts it automatically; check Settings → Engine if this persists.{:else}Start it with <code class="font-mono">ufoundry engine</code>.{/if}
      </span>
      <button class="ml-auto btn-outline btn-sm" onclick={() => app.rpc.retry()}>Retry now</button>
    {/if}
  </div>
{/if}
