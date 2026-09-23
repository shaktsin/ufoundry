import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

// In the Mac app the Go shell answers /__ufoundry/connection. During `npm run dev`
// this plugin does the same from the local engine's token file, so the UI can be
// developed in a normal browser against a running `ufoundry engine`.
function engineConnection(): Plugin {
  return {
    name: 'ufoundry-engine-connection',
    configureServer(server) {
      server.middlewares.use('/__ufoundry/connection', (_req, res) => {
        const home = process.env.UFOUNDRY_HOME || join(homedir(), '.ufoundry');
        const port = process.env.UFOUNDRY_ENGINE_WS_PORT || '8766';
        res.setHeader('Content-Type', 'application/json');
        res.setHeader('Cache-Control', 'no-store');
        try {
          const token = readFileSync(join(home, 'run', 'token'), 'utf8').trim();
          res.end(JSON.stringify({ url: `ws://127.0.0.1:${port}/ws`, token, shell: 'browser' }));
        } catch (err) {
          res.statusCode = 503;
          res.end(JSON.stringify({ error: `engine token not found in ${home}/run/token — is \`ufoundry engine\` running?` }));
        }
      });
    },
  };
}

export default defineConfig({
  plugins: [tailwindcss(), svelte(), engineConnection()],
  resolve: { alias: { $lib: '/src/lib' } },
  server: { port: 5173, strictPort: true, host: '127.0.0.1' },
  build: { outDir: 'dist', emptyOutDir: true, target: 'safari16' },
});
