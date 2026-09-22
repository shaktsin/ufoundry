<script lang="ts">
  import Sidebar from '$lib/components/Sidebar.svelte';
  import ConnectionBanner from '$lib/components/ConnectionBanner.svelte';
  import Toasts from '$lib/components/Toasts.svelte';
  import Dialog from '$lib/components/Dialog.svelte';
  import ChatView from '$lib/panels/ChatView.svelte';
  import Approvals from '$lib/panels/Approvals.svelte';
  import Tasks from '$lib/panels/Tasks.svelte';
  import Extensions from '$lib/panels/Extensions.svelte';
  import Usage from '$lib/panels/Usage.svelte';
  import Settings from '$lib/panels/Settings.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';

  function onKey(e: KeyboardEvent) {
    const mod = e.metaKey || e.ctrlKey;
    if (!mod) return;
    if (e.key === 'n') {
      e.preventDefault();
      chat.newChat();
    } else if (e.key === ',') {
      e.preventDefault();
      app.view = 'settings';
    } else if (e.key === 'k') {
      e.preventDefault();
      app.view = 'chat';
      document.getElementById('thread-search')?.focus();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="flex h-full overflow-hidden">
  <Sidebar />
  <div class="flex flex-col flex-1 min-w-0">
    <ConnectionBanner />
    <div class="flex-1 min-h-0 flex flex-col">
      {#if app.view === 'chat'}
        <ChatView />
      {:else}
        <main class="flex-1 overflow-y-auto">
          <div class="max-w-5xl mx-auto px-6 py-6">
            {#if app.view === 'approvals'}<Approvals />
            {:else if app.view === 'tasks'}<Tasks />
            {:else if app.view === 'extensions'}<Extensions />
            {:else if app.view === 'usage'}<Usage />
            {:else if app.view === 'settings'}<Settings />
            {/if}
          </div>
        </main>
      {/if}
    </div>
  </div>
</div>
<Toasts />
<Dialog />
