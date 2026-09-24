import { app } from './app.svelte';
import { projects } from './projects.svelte';
import { errMsg } from '$lib/format';
import type { Attachment, Item, ModelRouteResult, ModelSelection, RouteChangedEvent, SearchHit, Thread, Turn } from '$lib/types';

function sortThreads(list: Thread[]): Thread[] {
  return [...list].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return b.updatedAt.localeCompare(a.updatedAt);
  });
}

/** One open conversation: the main chat, or a side chat beside it. */
export class ThreadView {
  id = $state<string | null>(null);
  thread = $state<Thread | null>(null);
  turns = $state<Turn[]>([]);
  items = $state<Item[]>([]);
  loading = $state(false);
  sending = $state(false);
  selection = $state<ModelSelection>({});
  /** What this side chat was opened about, shown above the first message. */
  context = $state('');
  highlightItem = $state<string | null>(null);
  /** What the next message would run on, and what it would fall back to. */
  route = $state<ModelRouteResult | null>(null);
  private routeSeq = 0;

  get running(): Turn | undefined {
    return this.turns.find((t) => t.status === 'running');
  }

  get title(): string {
    return this.thread?.title || (this.id ? 'Untitled' : 'New chat');
  }

  reset() {
    this.id = null;
    this.thread = null;
    this.turns = [];
    this.items = [];
    this.sending = false;
    this.context = '';
    this.highlightItem = null;
  }

  async load(id: string) {
    this.id = id;
    this.loading = true;
    try {
      const r = await app.call<{ thread: Thread; turns: Turn[]; items: Item[] }>('thread/read', { threadId: id });
      if (this.id !== id) return;
      this.thread = r.thread;
      this.turns = r.turns ?? [];
      this.items = (r.items ?? []).sort((a, b) => a.seq - b.seq);
      this.selection = { ...(r.thread.settings ?? {}) };
      this.sending = this.turns.some((t) => t.status === 'running');
      void this.loadRoute();
    } catch (e) {
      app.toast('error', errMsg(e));
    } finally {
      this.loading = false;
    }
  }

  applyTurn(t: Turn) {
    if (t.threadId !== this.id) return;
    const i = this.turns.findIndex((x) => x.id === t.id);
    if (i >= 0) this.turns[i] = t;
    else this.turns = [...this.turns, t];
    if (t.status !== 'running') this.sending = this.turns.some((x) => x.status === 'running');
  }

  applyItem(it: Item) {
    if (it.threadId !== this.id) return;
    const i = this.items.findIndex((x) => x.id === it.id);
    if (i >= 0) {
      const prev = this.items[i];
      if (!it.text && prev.text) it.text = prev.text;
      this.items[i] = it;
    } else {
      this.items = [...this.items, it].sort((a, b) => a.seq - b.seq);
    }
  }

  applyDelta(itemId: string, text?: string, output?: string) {
    const it = this.items.find((x) => x.id === itemId);
    if (!it) return;
    if (text) it.text = (it.text || '') + text;
    if (output && it.tool) it.tool.output = (it.tool.output || '') + output;
  }

  async send(text: string, attachments: Attachment[] = [], start?: () => Promise<Thread>) {
    text = text.trim();
    if (!text && attachments.length === 0) return;
    this.sending = true;
    try {
      if (!this.id) {
        if (!start) throw new Error('no chat to send to');
        const th = await start();
        this.id = th.id;
        this.thread = th;
        this.turns = [];
        this.items = [];
      }
      await app.call('turn/start', { threadId: this.id, text, attachments });
    } catch (e) {
      this.sending = false;
      app.toast('error', errMsg(e));
    }
  }

  async interrupt() {
    const t = this.running;
    if (t) await app.try('turn/interrupt', { turnId: t.id });
  }

  async setSelection(sel: ModelSelection) {
    this.selection = sel;
    if (this.id) await app.try('thread/setSettings', { threadId: this.id, settings: sel });
    void this.loadRoute();
  }

  /**
   * Asks the engine what the next message would use. It is a preview, so a
   * failure is not worth a toast: the chip simply says nothing.
   */
  async loadRoute() {
    const seq = ++this.routeSeq;
    try {
      const r = await app.call<ModelRouteResult>('model/route', {
        threadId: this.id ?? undefined,
        projectId: projects.activeId ?? undefined,
        override: this.selection,
      });
      if (seq === this.routeSeq) this.route = r;
    } catch {
      if (seq === this.routeSeq) this.route = null;
    }
  }
}

class ChatState {
  threads = $state<Thread[]>([]);
  showArchived = $state(false);
  query = $state('');
  hits = $state<SearchHit[]>([]);
  /** The conversation in column 1. */
  main = new ThreadView();
  /** Side chats in column 2, most recent last. */
  sides = $state<ThreadView[]>([]);
  activeSideId = $state<string | null>(null);

  constructor() {
    const r = app.rpc;
    r.on('thread/updated', (p: { thread: Thread }) => this.upsertThread(p.thread));
    r.on('turn/started', (p: { turn: Turn }) => this.each((v) => v.applyTurn(p.turn)));
    r.on('turn/completed', (p: { turn: Turn }) => {
      this.each((v) => v.applyTurn(p.turn));
      // A turn can leave a key cooling down, so what the next one would use
      // may have changed.
      void this.main.loadRoute();
    });
    r.on('turn/routeChanged', (p: RouteChangedEvent) => {
      this.each((v) => {
        if (v.id === p.threadId) void v.loadRoute();
      });
    });
    r.on('item/started', (p: { item: Item }) => this.each((v) => v.applyItem(p.item)));
    r.on('item/completed', (p: { item: Item }) => this.each((v) => v.applyItem(p.item)));
    r.on('item/delta', (p: { threadId: string; itemId: string; text?: string; output?: string }) => {
      this.each((v) => {
        if (v.id === p.threadId) v.applyDelta(p.itemId, p.text, p.output);
      });
    });
    app.onConnected(() => this.reload());
  }

  private each(fn: (v: ThreadView) => void) {
    fn(this.main);
    for (const s of this.sides) fn(s);
  }

  get activeSide(): ThreadView | undefined {
    return this.sides.find((s) => s.id === this.activeSideId);
  }

  async reload() {
    await this.loadThreads();
    if (this.main.id) await this.main.load(this.main.id);
    else void this.main.loadRoute();
  }

  /** All chats, grouped by project in the rail. */
  async loadThreads() {
    try {
      const params: Record<string, unknown> = { archived: this.showArchived, limit: 200 };
      const r = await app.call<{ threads: Thread[] }>('thread/list', params);
      this.threads = sortThreads(r.threads ?? []);
    } catch (e) {
      app.toast('error', errMsg(e));
    }
  }

  private upsertThread(t: Thread) {
    const rest = this.threads.filter((x) => x.id !== t.id);
    this.threads = sortThreads(t.archived === this.showArchived ? [...rest, t] : rest);
    if (this.main.id === t.id) this.main.thread = t;
    for (const s of this.sides) if (s.id === t.id) s.thread = t;
  }

  /** Chats that are side chats of another chat, so the list can nest them. */
  sideChatsOf(threadId: string): Thread[] {
    return this.threads.filter((t) => t.forkedFrom === threadId);
  }

  get rootThreads(): Thread[] {
    const ids = new Set(this.threads.map((t) => t.id));
    return this.threads.filter((t) => !t.forkedFrom || !ids.has(t.forkedFrom));
  }

  async open(id: string, highlightItem?: string) {
    app.view = 'chat';
    const known = this.threads.find((t) => t.id === id);
    const targetProject = known?.projectId ?? null;
    if (known && projects.activeId !== targetProject) await projects.open(targetProject);
    this.main.highlightItem = highlightItem ?? null;
    await this.main.load(id);
    const loadedProject = this.main.thread?.projectId ?? null;
    if (projects.activeId !== loadedProject) await projects.open(loadedProject);
  }

  newChat() {
    app.view = 'chat';
    this.main.reset();
  }

  async newChatFor(projectId: string | null) {
    if (projects.activeId !== projectId) await projects.open(projectId);
    this.newChat();
  }

  /** Send in the main chat, creating the thread in the open project on first send. */
  async send(text: string, attachments: Attachment[] = []) {
    await this.main.send(text, attachments, async () => {
      const th = await app.call<Thread>('thread/start', {
        channel: 'app',
        projectId: projects.activeId ?? undefined,
        settings: this.main.selection,
      });
      this.upsertThread(th);
      return th;
    });
  }

  // ---- side chats ----

  /** Start a side chat about something in the main chat. */
  async startSide(context: string, question = '') {
    if (!this.main.id) {
      app.toast('warn', 'Open a chat first, then ask about one of its messages.');
      return;
    }
    const view = new ThreadView();
    view.context = context;
    view.selection = { ...this.main.selection };
    this.sides = [...this.sides, view];
    this.activeSideId = null;
    const th = await app.try<Thread>('thread/start', {
      channel: 'app',
      projectId: projects.activeId ?? undefined,
      parentThreadId: this.main.id,
      title: 'Side chat',
      settings: view.selection,
    });
    if (!th) {
      this.sides = this.sides.filter((s) => s !== view);
      return;
    }
    view.id = th.id;
    view.thread = th;
    this.activeSideId = th.id;
    this.upsertThread(th);
    const opener = [context && `About this:\n\n${context}`, question].filter(Boolean).join('\n\n');
    if (question) await view.send(opener);
    return view;
  }

  /** Bring an existing side chat back into column 2. */
  async openSide(id: string, context = '') {
    const existing = this.sides.find((s) => s.id === id);
    if (existing) {
      this.activeSideId = id;
      return existing;
    }
    const view = new ThreadView();
    view.context = context;
    this.sides = [...this.sides, view];
    this.activeSideId = id;
    await view.load(id);
    return view;
  }

  closeSide(id: string) {
    this.sides = this.sides.filter((s) => s.id !== id);
    if (this.activeSideId === id) this.activeSideId = this.sides.at(-1)?.id ?? null;
  }

  // ---- thread management ----

  async rename(id: string, title: string) {
    await app.try('thread/rename', { threadId: id, title });
  }

  async pin(t: Thread) {
    await app.try('thread/pin', { threadId: t.id, value: !t.pinned });
  }

  async archive(t: Thread) {
    await app.try('thread/archive', { threadId: t.id, value: !t.archived });
    if (this.main.id === t.id && !t.archived) this.newChat();
  }

  async remove(t: Thread) {
    const r = await app.try('thread/delete', { threadId: t.id });
    if (r === undefined) return;
    this.threads = this.threads.filter((x) => x.id !== t.id);
    if (this.main.id === t.id) this.newChat();
    this.closeSide(t.id);
  }

  async fork(t: Thread, upToItemId?: string) {
    const r = await app.try<Thread>('thread/fork', { threadId: t.id, upToItemId });
    if (r) await this.open(r.id);
  }

  async exportMarkdown(t: Thread): Promise<string | undefined> {
    const r = await app.try<{ markdown: string }>('thread/export', { threadId: t.id });
    return r?.markdown;
  }

  async search(q: string) {
    this.query = q;
    if (!q.trim()) {
      this.hits = [];
      return;
    }
    try {
      const r = await app.call<{ hits: SearchHit[] }>('thread/search', { query: q, limit: 30 });
      if (this.query === q) this.hits = r.hits ?? [];
    } catch {
      this.hits = [];
    }
  }

  async toggleArchived() {
    this.showArchived = !this.showArchived;
    await this.loadThreads();
  }
}

export const chat = new ChatState();
