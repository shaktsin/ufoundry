import { app } from '$lib/stores/app.svelte';
import { dialog } from '$lib/stores/dialog.svelte';
import { projects } from '$lib/stores/projects.svelte';

/** Collect a display name and folder, then register the folder as a project. */
export async function createProject() {
  const name = await dialog.prompt('Name this project', { okLabel: 'Choose folder' });
  if (!name?.trim()) return;

  let path = '';
  if (app.shell === 'mac') {
    try {
      const response = await fetch('/__ufoundry/shell/chooseFolder', { method: 'POST' });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
      path = result.path;
    } catch (error) {
      app.toast('error', error instanceof Error ? error.message : 'Could not open the folder picker.');
      return;
    }
  } else {
    const entered = await dialog.prompt('Enter the full path to the project folder', { okLabel: 'Open project' });
    if (!entered?.trim()) return;
    path = entered.trim();
  }

  if (!path.trim()) return;

  const project = await projects.add(path, name.trim());
  return project;
}
