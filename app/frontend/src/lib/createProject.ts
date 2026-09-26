import { dialog } from '$lib/stores/dialog.svelte';
import { projects } from '$lib/stores/projects.svelte';
import type { Project } from '$lib/types';

/** Collect a display name and folder, then register the folder as a project. */
export async function createProject() {
  const draft = await dialog.project();
  if (!draft) return;
  const project = await projects.add(draft.root, draft.name);
  return project;
}

/** Edit a project's display name and, when safe, its folder. */
export async function editProject(project: Project) {
  const draft = await dialog.project(project.name, project.root, project.threads === 0);
  if (!draft) return;
  const name = draft.name.trim();
  const root = draft.root.trim();
  if (name === project.name && root === project.root) return;
  await projects.update(project.id, { name, ...(root !== project.root ? { root } : {}) });
}
