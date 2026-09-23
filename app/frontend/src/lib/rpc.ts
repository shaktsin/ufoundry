// JSON-RPC 2.0 client for the UFoundry engine protocol over WebSocket.
// Reconnects with backoff; re-runs the handshake (initialize + subscribe) each time.

export const PROTOCOL_VERSION = '1.0.0';

export class RpcError extends Error {
  code: number;
  data?: unknown;
  constructor(code: number, message: string, data?: unknown) {
    super(message);
    this.code = code;
    this.data = data;
  }
}

export type ConnState = 'connecting' | 'open' | 'closed';
type Handler = (params: any) => void;

interface Pending {
  resolve: (v: any) => void;
  reject: (e: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

export interface Endpoint {
  url: string;
  token: string;
}

export class RpcClient {
  private ws: WebSocket | null = null;
  private nextId = 1;
  private pending = new Map<number, Pending>();
  private handlers = new Map<string, Set<Handler>>();
  private stateHandlers = new Set<(s: ConnState, err?: string) => void>();
  private backoff = 500;
  private stopped = false;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  state: ConnState = 'closed';
  lastError = '';

  constructor(
    private endpoint: () => Promise<Endpoint>,
    private onOpen: (c: RpcClient) => Promise<void>,
    private socketFactory: (url: string) => WebSocket = (u) => new WebSocket(u),
  ) {}

  start() {
    this.stopped = false;
    void this.connect();
  }

  stop() {
    this.stopped = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.ws?.close();
  }

  onState(fn: (s: ConnState, err?: string) => void): () => void {
    this.stateHandlers.add(fn);
    return () => this.stateHandlers.delete(fn);
  }

  on(method: string, fn: Handler): () => void {
    let set = this.handlers.get(method);
    if (!set) this.handlers.set(method, (set = new Set()));
    set.add(fn);
    return () => set!.delete(fn);
  }

  private setState(s: ConnState, err = '') {
    this.state = s;
    this.lastError = err;
    for (const fn of this.stateHandlers) fn(s, err);
  }

  private async connect() {
    if (this.stopped) return;
    this.setState('connecting', this.lastError);
    let ep: Endpoint;
    try {
      ep = await this.endpoint();
    } catch (e) {
      this.scheduleReconnect(e instanceof Error ? e.message : String(e));
      return;
    }
    const url = ep.url + (ep.url.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(ep.token);
    let ws: WebSocket;
    try {
      ws = this.socketFactory(url);
    } catch (e) {
      this.scheduleReconnect(e instanceof Error ? e.message : String(e));
      return;
    }
    this.ws = ws;
    let opened = false;
    ws.onopen = async () => {
      opened = true;
      try {
        await this.onOpen(this);
        this.backoff = 500;
        this.setState('open');
      } catch (e) {
        this.lastError = e instanceof Error ? e.message : String(e);
        ws.close();
      }
    };
    ws.onmessage = (ev) => this.handle(typeof ev.data === 'string' ? ev.data : '');
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      this.failPending(new RpcError(-32000, 'connection closed'));
      this.scheduleReconnect(opened ? this.lastError || 'connection lost' : 'engine not reachable');
    };
    ws.onerror = () => {
      /* onclose follows */
    };
  }

  private scheduleReconnect(err: string) {
    this.setState('closed', err);
    if (this.stopped) return;
    const delay = this.backoff;
    this.backoff = Math.min(this.backoff * 2, 10_000);
    this.reconnectTimer = setTimeout(() => void this.connect(), delay);
  }

  /** Reconnect now (e.g. after the user starts the engine). */
  retry() {
    if (this.state !== 'closed') return;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.backoff = 500;
    void this.connect();
  }

  private failPending(err: Error) {
    for (const [, p] of this.pending) {
      clearTimeout(p.timer);
      p.reject(err);
    }
    this.pending.clear();
  }

  private handle(data: string) {
    let msg: any;
    try {
      msg = JSON.parse(data);
    } catch {
      return;
    }
    if (msg.id != null && (msg.result !== undefined || msg.error !== undefined) && !msg.method) {
      const p = this.pending.get(msg.id);
      if (!p) return;
      this.pending.delete(msg.id);
      clearTimeout(p.timer);
      if (msg.error) p.reject(new RpcError(msg.error.code, msg.error.message, msg.error.data));
      else p.resolve(msg.result);
      return;
    }
    if (msg.method) {
      const set = this.handlers.get(msg.method);
      if (set) for (const fn of set) {
        try {
          fn(msg.params);
        } catch (e) {
          console.error('handler', msg.method, e);
        }
      }
      const any = this.handlers.get('*');
      if (any) for (const fn of any) fn(msg);
    }
  }

  call<T = any>(method: string, params?: unknown, timeoutMs = 120_000): Promise<T> {
    const ws = this.ws;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      return Promise.reject(new RpcError(-32000, 'not connected to the engine'));
    }
    const id = this.nextId++;
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new RpcError(-32000, `${method} timed out`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      ws.send(JSON.stringify({ jsonrpc: '2.0', id, method, params: params ?? {} }));
    });
  }
}
