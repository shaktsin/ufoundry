import type { Endpoint } from './rpc';

export interface ConnectionInfo extends Endpoint {
  /** "mac" when running inside the UFoundry app, "browser" for the dev server. */
  shell: string;
  version?: string;
}

/**
 * Where to reach the engine. The Mac app's Go shell and the Vite dev server both
 * answer /__ufoundry/connection with the WebSocket URL and bearer token (read
 * from ~/.ufoundry/run/token). A #token=…&url=… fragment overrides it, which is
 * handy for pointing a browser at another engine.
 */
export async function fetchConnection(): Promise<ConnectionInfo> {
  const frag = new URLSearchParams(location.hash.replace(/^#/, ''));
  if (frag.get('token')) {
    return { url: frag.get('url') || 'ws://127.0.0.1:8766/ws', token: frag.get('token')!, shell: 'browser' };
  }
  const res = await fetch('/__ufoundry/connection', { cache: 'no-store' });
  let body: any = {};
  try {
    body = await res.json();
  } catch {
    /* not JSON */
  }
  if (!res.ok || !body.token) {
    throw new Error(body.error || `engine connection info unavailable (HTTP ${res.status})`);
  }
  return body as ConnectionInfo;
}
