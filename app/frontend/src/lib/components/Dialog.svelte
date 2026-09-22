<script lang="ts">
  import { dialog } from '$lib/stores/dialog.svelte';

  function onKey(e: KeyboardEvent) {
    if (!dialog.current) return;
    if (e.key === 'Escape') dialog.close(false);
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      dialog.close(true);
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if dialog.current}
  {@const d = dialog.current}
  <div class="fixed inset-0 z-40 bg-black/50 flex items-center justify-center p-6" role="presentation" onclick={() => dialog.close(false)}>
    <div class="card w-full max-w-md p-5 shadow-2xl" role="dialog" aria-modal="true" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
      <p class="text-sm text-zinc-200 whitespace-pre-wrap">{d.message}</p>
      {#if d.kind === 'prompt'}
        <!-- svelte-ignore a11y_autofocus -->
        <input class="input mt-3 font-mono" type={d.secret ? 'password' : 'text'} autocomplete="off" bind:value={d.value} autofocus />
      {/if}
      <div class="flex justify-end gap-2 mt-4">
        <button class="btn-ghost" onclick={() => dialog.close(false)}>Cancel</button>
        <!-- svelte-ignore a11y_autofocus -->
        <button class={d.danger ? 'btn bg-red-600 hover:bg-red-500 text-white' : 'btn-primary'} autofocus={d.kind === 'confirm'} onclick={() => dialog.close(true)}>{d.okLabel}</button>
      </div>
    </div>
  </div>
{/if}
