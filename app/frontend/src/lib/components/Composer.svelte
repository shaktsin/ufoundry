<script lang="ts">
  import { ArrowUp, Square, Paperclip, X } from '@lucide/svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { app } from '$lib/stores/app.svelte';
  import ModelPicker from './ModelPicker.svelte';
  import type { Attachment } from '$lib/types';

  let text = $state('');
  let files = $state<Attachment[]>([]);
  let input: HTMLTextAreaElement | undefined = $state();
  let fileInput: HTMLInputElement | undefined = $state();

  const MAX_FILE = 15 * 1024 * 1024;
  const busy = $derived(chat.sending || !!chat.running);
  const offline = $derived(app.conn !== 'open');

  export function focus() {
    input?.focus();
  }

  function submit() {
    if (busy || offline) return;
    const t = text;
    const a = files;
    if (!t.trim() && a.length === 0) return;
    text = '';
    files = [];
    void chat.send(t, a);
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      submit();
    }
  }

  function readFile(f: File): Promise<Attachment> {
    return new Promise((resolve, reject) => {
      const r = new FileReader();
      r.onload = () => {
        const s = String(r.result);
        resolve({ name: f.name, mimeType: f.type || 'application/octet-stream', dataB64: s.slice(s.indexOf(',') + 1) });
      };
      r.onerror = () => reject(r.error);
      r.readAsDataURL(f);
    });
  }

  async function addFiles(list: FileList | File[] | null) {
    if (!list) return;
    for (const f of Array.from(list)) {
      if (f.size > MAX_FILE) {
        app.toast('error', `${f.name} is larger than 15 MB.`);
        continue;
      }
      files = [...files, await readFile(f)];
    }
  }

  function onPaste(e: ClipboardEvent) {
    const fl = e.clipboardData?.files;
    if (fl && fl.length) {
      e.preventDefault();
      void addFiles(fl);
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    void addFiles(e.dataTransfer?.files ?? null);
  }
</script>

<div class="px-4 pb-4 pt-2 shrink-0">
  <div
    class="max-w-3xl mx-auto bg-zinc-900 border border-zinc-800 rounded-2xl focus-within:border-zinc-600 transition-colors"
    role="region"
    aria-label="Message composer"
    ondragover={(e) => e.preventDefault()}
    ondrop={onDrop}
  >
    {#if files.length}
      <div class="flex flex-wrap gap-1.5 px-3 pt-2.5">
        {#each files as f, i}
          <span class="inline-flex items-center gap-1 text-xs bg-zinc-800 rounded-md pl-2 pr-1 py-0.5">
            {f.name}
            <button aria-label={`Remove ${f.name}`} class="text-zinc-500 hover:text-zinc-200" onclick={() => (files = files.filter((_, j) => j !== i))}>
              <X class="w-3 h-3" />
            </button>
          </span>
        {/each}
      </div>
    {/if}
    <textarea
      bind:this={input}
      bind:value={text}
      onkeydown={onKeydown}
      onpaste={onPaste}
      rows="1"
      placeholder={offline ? 'Engine offline…' : 'Ask UFoundry anything'}
      class="w-full bg-transparent px-4 pt-3 pb-1 text-sm resize-none focus:outline-none placeholder:text-zinc-500 max-h-60"
      style="field-sizing: content; min-height: 2.75rem"
    ></textarea>
    <div class="flex items-center gap-2 px-2.5 pb-2.5">
      <button class="btn-ghost btn-sm" title="Attach files" aria-label="Attach files" onclick={() => fileInput?.click()}>
        <Paperclip class="w-4 h-4" />
      </button>
      <input bind:this={fileInput} type="file" multiple class="hidden" onchange={(e) => { addFiles(e.currentTarget.files); e.currentTarget.value = ''; }} />
      <ModelPicker value={chat.selection} onchange={(v) => chat.setSelection(v)} />
      <div class="ml-auto">
        {#if busy}
          <button class="w-8 h-8 rounded-full bg-zinc-200 text-zinc-900 flex items-center justify-center hover:bg-white" title="Stop" aria-label="Stop" onclick={() => chat.interrupt()}>
            <Square class="w-3.5 h-3.5 fill-current" />
          </button>
        {:else}
          <button
            class="w-8 h-8 rounded-full bg-violet-600 text-white flex items-center justify-center hover:bg-violet-500 disabled:opacity-30"
            title="Send (Enter)"
            aria-label="Send"
            disabled={offline || (!text.trim() && files.length === 0)}
            onclick={submit}
          >
            <ArrowUp class="w-4 h-4" />
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>
