<script lang="ts">
  import { Check, Copy, RefreshCw, Terminal, TriangleAlert } from '@lucide/svelte';
  import { app } from '$lib/stores/app.svelte';

  let refreshing = $state(false);

  async function refresh() {
    refreshing = true;
    try {
      await app.refreshCatalog();
    } finally {
      refreshing = false;
    }
  }

  async function copy(command: string) {
    await navigator.clipboard.writeText(command);
    app.toast('info', 'Command copied. Run it in Terminal, then refresh status.');
  }
</script>

<div class="flex items-start gap-4 mb-4">
  <div>
    <h2 class="text-sm font-semibold text-ink">Subscription accounts</h2>
    <p class="text-xs text-muted mt-1 max-w-2xl">Use the official console sign-in from ChatGPT or Claude. ufoundry checks the session but never reads, copies, or stores its OAuth token.</p>
  </div>
  <button class="btn-ghost btn-sm ml-auto" disabled={refreshing} onclick={refresh}><RefreshCw class="w-3.5 h-3.5 {refreshing ? 'animate-spin' : ''}" />Refresh</button>
</div>

<div class="grid gap-3 md:grid-cols-2">
  {#each app.identities as identity (identity.id)}
    <section class="card p-4 flex flex-col min-h-40">
      <div class="flex items-start gap-3">
        <div class="w-9 h-9 rounded-lg bg-raised border border-line flex items-center justify-center text-muted"><Terminal class="w-4 h-4" /></div>
        <div class="min-w-0">
          <h3 class="text-sm font-semibold text-ink">{identity.displayName}</h3>
          <p class="text-xs text-muted">via {identity.runtimeName}</p>
        </div>
        <span class="ml-auto inline-flex items-center gap-1.5 text-xs {identity.signedIn ? 'text-sage' : 'text-muted'}">
          {#if identity.signedIn}<Check class="w-3.5 h-3.5" />{:else}<TriangleAlert class="w-3.5 h-3.5" />{/if}
          {identity.signedIn ? 'Connected' : identity.installed ? 'Not connected' : 'Not installed'}
        </span>
      </div>
      <p class="text-xs text-ink-soft mt-4">{identity.status}</p>
      {#if identity.accountType}<p class="text-[11px] text-muted mt-1">{identity.accountType}</p>{/if}
      <div class="mt-auto pt-4 flex gap-2">
        <button class="btn-outline btn-sm" onclick={() => copy(identity.signedIn ? identity.signOutCommand : identity.signInCommand)} disabled={!identity.installed}>
          <Copy class="w-3.5 h-3.5" />{identity.signedIn ? 'Copy sign-out command' : 'Copy sign-in command'}
        </button>
      </div>
    </section>
  {/each}
</div>

<div class="card p-4 mt-5">
  <h2 class="text-sm font-semibold text-ink">How these accounts will be used</h2>
  <p class="text-xs text-muted mt-1 max-w-3xl">Subscription accounts are separate from metered API keys. Once a managed runtime is enabled for routing, it can be placed in the same model pool and selected when another route reaches a provider or organization quota.</p>
</div>
