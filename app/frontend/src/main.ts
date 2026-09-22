import './app.css';
import { mount } from 'svelte';
import App from './App.svelte';
import { app } from '$lib/stores/app.svelte';
import { chat } from '$lib/stores/chat.svelte';

// Hooks the Go shell calls (window.ExecJS) from the menu-bar menu and notifications.
declare global {
  interface Window {
    ufoundry: {
      newChat: () => void;
      show: (view: string) => void;
      openThread: (id: string) => void;
      reconnect: () => void;
    };
  }
}
window.ufoundry = {
  newChat: () => chat.newChat(),
  show: (view) => (app.view = view as typeof app.view),
  openThread: (id) => void chat.open(id),
  reconnect: () => app.rpc.retry(),
};

app.start();
export default mount(App, { target: document.getElementById('app')! });
