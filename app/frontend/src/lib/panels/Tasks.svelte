<script lang="ts">
  import { Plus, Play, Ban, RefreshCw } from '@lucide/svelte';
  import { app } from '$lib/stores/app.svelte';
  import { chat } from '$lib/stores/chat.svelte';
  import { fmtDateTime, errMsg } from '$lib/format';
  import ModelPicker from '$lib/components/ModelPicker.svelte';
  import { dialog } from '$lib/stores/dialog.svelte';
  import type { ModelSelection, Schedule, Task, TaskRun } from '$lib/types';

  let tasks = $state<Task[]>([]);
  let filter = $state<'active' | ''>('active');
  let creating = $state(false);
  let runsFor = $state<number | null>(null);
  let runs = $state<TaskRun[]>([]);

  // New task form
  let name = $state('');
  let prompt = $state('');
  let kind = $state<'one_time' | 'periodic'>('periodic');
  let runAt = $state('');
  let frequency = $state<'hourly' | 'daily' | 'weekly' | 'cron'>('daily');
  let time = $state('09:00');
  let minute = $state(0);
  let dow = $state('mon');
  let cron = $state('');
  let settings = $state<ModelSelection>({});
  let saving = $state(false);

  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;

  async function load() {
    const r = await app.try<{ tasks: Task[] }>('task/list', { status: filter });
    if (r) tasks = r.tasks;
  }

  $effect(() => {
    void filter;
    if (app.conn === 'open') void load();
  });

  $effect(() => app.rpc.on('task/updated', (p: { task: Task }) => {
    const i = tasks.findIndex((t) => t.id === p.task.id);
    const show = !filter || p.task.status === filter;
    if (i >= 0) {
      if (show) tasks[i] = p.task;
      else tasks = tasks.filter((t) => t.id !== p.task.id);
    } else if (show) tasks = [p.task, ...tasks];
    if (runsFor === p.task.id) void showRuns(p.task.id, true);
  }));

  function scheduleText(t: Task): string {
    const s = t.schedule;
    if (t.taskType === 'one_time') return `Once at ${s.run_at}`;
    switch (s.frequency) {
      case 'hourly': return `Hourly at :${String(s.minute ?? 0).padStart(2, '0')}`;
      case 'daily': return `Daily at ${s.time}`;
      case 'weekly': return `Weekly on ${s.day_of_week} at ${s.time}`;
      case 'cron': return `Cron ${s.cron}`;
    }
    return JSON.stringify(s);
  }

  async function create() {
    let schedule: Schedule;
    if (kind === 'one_time') schedule = { run_at: runAt };
    else if (frequency === 'hourly') schedule = { frequency, minute };
    else if (frequency === 'daily') schedule = { frequency, time };
    else if (frequency === 'weekly') schedule = { frequency, day_of_week: dow, time };
    else schedule = { frequency, cron };
    saving = true;
    try {
      await app.call('task/create', { name, prompt, taskType: kind, schedule, timezone: tz, settings });
      app.toast('info', `Scheduled “${name}”.`);
      creating = false;
      name = prompt = cron = runAt = '';
      await load();
    } catch (e) {
      app.toast('error', errMsg(e));
    } finally {
      saving = false;
    }
  }

  async function showRuns(id: number, keepOpen = false) {
    if (runsFor === id && !keepOpen) {
      runsFor = null;
      return;
    }
    runsFor = id;
    const r = await app.try<{ runs: TaskRun[] }>('task/runs', { taskId: id });
    runs = r?.runs ?? [];
  }

  async function cancel(t: Task) {
    if (!(await dialog.confirm(`Cancel “${t.name}”? It will not run again.`, { okLabel: 'Cancel task', danger: true }))) return;
    await app.try('task/cancel', { taskId: t.id }, 'Task cancelled.');
    await load();
  }
</script>

<div class="flex items-center gap-3 mb-4">
  <h1 class="page-title">Scheduled tasks</h1>
  <select class="select" bind:value={filter}>
    <option value="active">Active</option>
    <option value="">All</option>
  </select>
  <button class="btn-ghost btn-sm" aria-label="Refresh" onclick={load}><RefreshCw class="w-3.5 h-3.5" /></button>
  <button class="btn-primary ml-auto" onclick={() => (creating = !creating)}><Plus class="w-4 h-4" />New task</button>
</div>

{#if creating}
  <div class="card p-4 mb-4 space-y-3">
    <div class="grid grid-cols-2 gap-3">
      <div><label class="label" for="t-name">Name</label><input id="t-name" class="input" bind:value={name} placeholder="Morning briefing" /></div>
      <div>
        <label class="label" for="t-kind">Runs</label>
        <select id="t-kind" class="input" bind:value={kind}><option value="periodic">Repeatedly</option><option value="one_time">Once</option></select>
      </div>
    </div>
    <div><label class="label" for="t-prompt">Prompt</label><textarea id="t-prompt" class="input" rows="3" bind:value={prompt} placeholder="Summarize my unread email and today's calendar."></textarea></div>
    {#if kind === 'one_time'}
      <div class="w-64"><label class="label" for="t-at">At ({tz})</label><input id="t-at" type="datetime-local" class="input" bind:value={runAt} /></div>
    {:else}
      <div class="flex gap-3 items-end">
        <div><label class="label" for="t-freq">Every</label>
          <select id="t-freq" class="input" bind:value={frequency}>
            <option value="hourly">Hour</option><option value="daily">Day</option><option value="weekly">Week</option><option value="cron">Cron expression</option>
          </select>
        </div>
        {#if frequency === 'hourly'}
          <div class="w-28"><label class="label" for="t-min">Minute</label><input id="t-min" type="number" min="0" max="59" class="input" bind:value={minute} /></div>
        {:else if frequency === 'cron'}
          <div class="flex-1"><label class="label" for="t-cron">Cron (min hour dom mon dow)</label><input id="t-cron" class="input font-mono" bind:value={cron} placeholder="*/30 9-17 * * mon-fri" /></div>
        {:else}
          {#if frequency === 'weekly'}
            <div><label class="label" for="t-dow">Day</label>
              <select id="t-dow" class="input" bind:value={dow}>
                {#each ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as d}<option value={d}>{d}</option>{/each}
              </select>
            </div>
          {/if}
          <div class="w-32"><label class="label" for="t-time">Time ({tz})</label><input id="t-time" type="time" class="input" bind:value={time} /></div>
        {/if}
      </div>
    {/if}
    <div><span class="label">Model and complexity</span><ModelPicker value={settings} onchange={(v) => (settings = v)} /></div>
    <div class="flex justify-end gap-2">
      <button class="btn-ghost" onclick={() => (creating = false)}>Cancel</button>
      <button class="btn-primary" disabled={saving || !name.trim() || !prompt.trim()} onclick={create}>Schedule</button>
    </div>
  </div>
{/if}

{#if tasks.length === 0}
  <div class="card p-8 text-center text-sm text-muted">No {filter === 'active' ? 'active ' : ''}tasks. You can also ask in chat: “every weekday at 9, summarize my inbox”.</div>
{:else}
  <div class="card overflow-hidden">
    <table class="data-table">
      <thead><tr><th>Task</th><th>Schedule</th><th>Next run</th><th>Last run</th><th></th></tr></thead>
      <tbody>
        {#each tasks as t (t.id)}
          <tr>
            <td class="max-w-xs">
              <div class="text-ink">{t.name}</div>
              <div class="text-xs text-muted truncate" title={t.prompt}>{t.prompt}</div>
              {#if t.status !== 'active'}<span class="pill bg-raised text-muted mt-1">{t.status}</span>{/if}
            </td>
            <td class="text-xs text-muted">{scheduleText(t)}<div class="text-faint">{t.timezone}</div></td>
            <td class="text-xs text-muted">{fmtDateTime(t.nextRunAt)}</td>
            <td class="text-xs">
              <span class="text-muted">{fmtDateTime(t.lastRunAt)}</span>
              {#if t.lastError}<div class="text-rust truncate max-w-48" title={t.lastError}>{t.lastError}</div>{/if}
            </td>
            <td class="text-right whitespace-nowrap">
              <button class="btn-ghost btn-sm" onclick={() => showRuns(t.id)}>History</button>
              {#if t.threadId}<button class="btn-ghost btn-sm" onclick={() => chat.open(t.threadId!)}>Chat</button>{/if}
              {#if t.status === 'active'}
                <button class="btn-ghost btn-sm" title="Run now" aria-label="Run now" onclick={() => app.try('task/runNow', { taskId: t.id }, 'Started.')}><Play class="w-3.5 h-3.5" /></button>
                <button class="btn-danger btn-sm" title="Cancel" aria-label="Cancel task" onclick={() => cancel(t)}><Ban class="w-3.5 h-3.5" /></button>
              {/if}
            </td>
          </tr>
          {#if runsFor === t.id}
            <tr><td colspan="5" class="bg-surface/60">
              {#if runs.length === 0}<p class="text-xs text-muted">No runs yet.</p>{/if}
              {#each runs as r (r.id)}
                <div class="flex gap-3 text-xs py-1">
                  <span class="w-32 text-muted">{fmtDateTime(r.startedAt)}</span>
                  <span class="w-16 {r.status === 'success' ? 'text-sage' : r.status === 'failed' ? 'text-rust' : 'text-amber-warm'}">{r.status}</span>
                  <span class="flex-1 text-muted truncate selectable" title={r.error || r.result}>{r.error || r.result}</span>
                </div>
              {/each}
            </td></tr>
          {/if}
        {/each}
      </tbody>
    </table>
  </div>
{/if}
