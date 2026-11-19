import { useCallback, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { getActiveWorkspaceId, setActiveWorkspaceId, workspacesService } from "../service";
import type { Workspace } from "../types/workspace";
import { WorkspaceContext, type WorkspaceContextValue } from "./WorkspaceContext";

const STORAGE_KEY = "beautyscale.workspace";

/** Reads the remembered workspace. Storage can throw in a private window or
 *  with site data blocked, so every access is guarded. */
function readStored(): Workspace | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;

    const parsed: unknown = JSON.parse(raw);
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      typeof (parsed as Workspace).id === "string"
    ) {
      return parsed as Workspace;
    }
  } catch {
    // Ignore: a missing or unreadable preference is not an error.
  }
  return null;
}

function writeStored(workspace: Workspace | null): void {
  try {
    if (workspace) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(workspace));
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    // Ignore: remembering the choice is a convenience, not a requirement.
  }
}

/**
 * The workspace to start as: the remembered one when it is still configured,
 * otherwise the first configured one.
 *
 * A remembered ID is dropped when the configured list no longer contains it,
 * because a stale choice (an older seed, a different database) would make
 * every request 404 against a tenant that is gone. With no list at all the
 * remembered ID is kept, since that is the pasted-ID mode.
 */
function resolveInitial(workspaces: Workspace[]): Workspace | null {
  const stored = readStored();
  if (stored) {
    if (workspaces.length === 0) return stored;
    const known = workspaces.find((option) => option.id === stored.id);
    if (known) return known;
  }
  return workspaces[0] ?? null;
}

/**
 * Holds the active workspace and keeps the service layer in step with it.
 *
 * Choosing a workspace changes which tenant the API answers for, so the query
 * cache is cleared on every change: query keys are workspace-scoped, but
 * clearing removes any chance of showing one tenant's rows under another.
 */
export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const workspaces = useMemo(() => workspacesService.list(), []);

  const [workspace, setWorkspace] = useState<Workspace | null>(() => {
    const initial = resolveInitial(workspaces);
    // Set during the initializer, not in an effect: React runs a child's
    // effects before its parent's, so a page's query would mount and fetch
    // with no header yet, and the API would answer 400 "X-Workspace-ID
    // header is required" on every reload.
    setActiveWorkspaceId(initial?.id ?? null);
    return initial;
  });

  // Keeps the module-level value in step if it ever drifts from state - a
  // StrictMode remount, or a second provider - before any child renders.
  // Idempotent, and derived only from state, so it is safe during render.
  if (getActiveWorkspaceId() !== (workspace?.id ?? null)) {
    setActiveWorkspaceId(workspace?.id ?? null);
  }

  const change = useCallback(
    (next: Workspace | null) => {
      setActiveWorkspaceId(next?.id ?? null);
      writeStored(next);
      setWorkspace(next);
      // Nothing cached under the previous tenant should be reused.
      queryClient.clear();
    },
    [queryClient],
  );

  const value = useMemo<WorkspaceContextValue>(
    () => ({
      workspace,
      workspaces,
      selectWorkspace: (next) => change(next),
      selectWorkspaceById: (id, name) =>
        change({ id: id.trim(), name: name?.trim() || "Workspace" }),
      clearWorkspace: () => change(null),
    }),
    [workspace, workspaces, change],
  );

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}
