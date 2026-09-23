<script lang="ts">
  import { FolderGit2, AlertTriangle, Save, Trash2 } from '@lucide/svelte';
  import { projects } from '$lib/stores/projects.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { inspector } from '$lib/stores/inspector.svelte';
  import { app } from '$lib/stores/app.svelte';
  import { dialog } from '$lib/stores/dialog.svelte';
  import ModelPicker from '$lib/components/ModelPicker.svelte';
  import DiffView from '$lib/components/DiffView.svelte';
  import type { FileChangeData, ModelSelection } from '$lib/types';

  let instructions = $state('');
  let dirty = $state(false);
  let changes = $state<FileChangeData[]>([]);
  const p = $derived(projects.active);

  $effect(() => {
    void projects.activeId;
    if (!projects.activeId) return;
    void projects.loadInstructions();
    void loadChanges();
  });

  $effect(() => {
    const text = projects.instructions?.project;
    if (text !== undefined && !dirty) instructions = text;
  });

  async function loadChanges() {
    const r = await app.try<{ files: FileChangeData[] }>('project/diff', { projectId: projects.activeId });
    changes = r?.files ?? [];
  }

  async function save() {
    await projects.saveInstructions(instructions);
    dirty = false;
  }

  async function setTool(key: 'shell' | 'network', value: boolean) {
    if (!p) return;
    await projects.update(p.id, { tools: { ...p.tools, [key]: value } });
  }

  async function setSettings(sel: ModelSelection) {
    if (!p) return;
    await projects.update(p.id, { settings: sel });
  }

  async function rename() {
    if (!p) return;
    const name = await dialog.prompt('New name for this project', { okLabel: 'Rename' });
    if (name?.trim()) await projects.update(p.id, { name: name.trim() });
  }

  async function close() {
    if (!p) return;
    const ok = await dialog.confirm(
      `Close the project “${p.name}”? Its chats stay, and nothing on disk is touched.`,
      { okLabel: 'Close project', danger: true },
    );
    if (ok) {
      await projects.remove(p.id);
      chat.newChat();
      await chat.loadThreads();
      app.view = 'chat';
    }
  }
</script>

{#if !p}
  <p class="text-sm text-muted">No project is open. Pick one from the switcher at the top left.</p>
{:else}
  <div class="flex items-center gap-3 mb-5">
    <div class="w-9 h-9 rounded-lg bg-clay text-white flex items-center justify-center font-semibold">
      {p.name.slice(0, 1).toUpperCase()}
    </div>
    <div class="min-w-0">
      <h1 class="page-title truncate">{p.name}</h1>
      <p class="text-xs text-muted font-mono truncate selectable">{p.root}</p>
    </div>
    <div class="ml-auto flex gap-2">
      <button class="btn-ghost btn-sm" onclick={rename}>Rename</button>
      <button class="btn-danger btn-sm" onclick={close}><Trash2 class="w-3.5 h-3.5" />Close project</button>
    </div>
  </div>

  {#if p.missing}
    <div class="card p-3 mb-4 flex items-center gap-2 text-sm text-rust">
      <AlertTriangle class="w-4 h-4" />This folder is gone from disk. Chats in it can still be read, but nothing can run.
    </div>
  {/if}

  <div class="grid grid-cols-2 gap-3 mb-5">
    <div class="card p-3">
      <div class="text-xs text-muted mb-1">Version control</div>
      {#if p.vcs}
        <div class="flex items-center gap-2 text-sm">
          <FolderGit2 class="w-4 h-4 text-muted" />
          <span>{p.vcs.branch || 'git'}</span>
          {#if p.vcs.dirty > 0}<span class="text-amber-warm">· {p.vcs.dirty} changed files</span>{/if}
        </div>
        {#if p.vcs.remote}<div class="text-[11px] text-faint truncate mt-1 selectable">{p.vcs.remote}</div>{/if}
      {:else}
        <div class="text-sm text-muted">Not a git repository.</div>
      {/if}
    </div>
    <div class="card p-3">
      <div class="text-xs text-muted mb-1">Chats in this project</div>
      <div class="text-sm">{p.threads}</div>
    </div>
  </div>

  <section class="card p-4 mb-5">
    <h2 class="text-sm font-semibold mb-3">What the agent may do here</h2>
    <div class="space-y-3 text-sm">
      <label class="flex items-center gap-3">
        <input type="checkbox" checked={p.tools.shell !== false} onchange={(e) => setTool('shell', e.currentTarget.checked)} />
        <span>
          Run shell commands
          <span class="block text-xs text-muted">They run in this folder, with no API keys in their environment.</span>
        </span>
      </label>
      <label class="flex items-center gap-3">
        <input type="checkbox" checked={!!p.tools.network} onchange={(e) => setTool('network', e.currentTarget.checked)} />
        <span>
          Let commands use the network
          <span class="block text-xs text-muted">Off by default: curl, git push, npm install and the like are refused.</span>
        </span>
      </label>
      <div>
        <span class="label">Default model for chats here</span>
        <ModelPicker value={p.settings} onchange={setSettings} />
      </div>
    </div>
  </section>

  <section class="card p-4 mb-5">
    <div class="flex items-center mb-2">
      <h2 class="text-sm font-semibold">Project instructions</h2>
      <span class="ml-2 text-xs text-muted font-mono truncate">{projects.instructions?.path ?? 'AGENT.md'}</span>
      <button class="btn-primary btn-sm ml-auto" disabled={!dirty} onclick={save}><Save class="w-3.5 h-3.5" />Save</button>
    </div>
    <p class="text-xs text-muted mb-2">
      Added to the agent's prompt for every chat in this project, after your global instructions. An existing
      <code class="font-mono">AGENTS.md</code> or <code class="font-mono">CLAUDE.md</code> is used as-is.
    </p>
    <textarea
      class="input font-mono text-xs h-48"
      bind:value={instructions}
      oninput={() => (dirty = true)}
      placeholder="Run the tests with `make test`. Prefer small commits. The API lives in internal/api."
    ></textarea>
    {#if projects.instructions?.sources?.length}
      <div class="mt-2 text-[11px] text-faint">
        In effect:
        {#each projects.instructions.sources as s, i}
          <span>{i > 0 ? ' → ' : ''}{s.scope} ({s.path.split('/').pop()})</span>
        {/each}
      </div>
    {/if}
  </section>

  <section>
    <div class="flex items-center mb-2">
      <h2 class="text-sm font-semibold">Changes the agent has made</h2>
      <button class="btn-ghost btn-sm ml-auto" onclick={loadChanges}>Refresh</button>
    </div>
    {#if changes.length === 0}
      <p class="text-sm text-muted">Nothing changed in this project yet.</p>
    {/if}
    <div class="space-y-2">
      {#each changes as c (c.path)}
        <div class="card p-3">
          <div class="flex items-center gap-2 text-xs mb-2">
            <button class="font-mono text-ink hover:underline" onclick={() => inspector.openFile(c.path)}>{c.path}</button>
            <span class="text-sage">+{c.additions}</span>
            <span class="text-rust">−{c.deletions}</span>
            <div class="ml-auto flex gap-2">
              <button class="text-faint hover:text-ink" onclick={() => inspector.openDiff(c)}>open</button>
              {#if c.turnId && c.revertable}
                <button class="text-faint hover:text-ink" onclick={() => inspector.revertTurn(c.turnId!, [c.path]).then(loadChanges)}>
                  undo
                </button>
              {/if}
            </div>
          </div>
          <DiffView change={c} max={24} />
        </div>
      {/each}
    </div>
  </section>
{/if}
