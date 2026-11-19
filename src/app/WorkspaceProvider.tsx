import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { setActiveWorkspaceId, workspacesService } from "../service";
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
    const stored = readStored();
    if (stored) return stored;
    // Default to the first configured workspace, so a fresh visit works.
    return workspaces[0] ?? null;
  });

  // The service layer attaches the header, so it must know the selection
  // before any query runs. Done in an effect so it is applied on first paint
  // and on every change, including a restored selection.
  useEffect(() => {
    setActiveWorkspaceId(workspace?.id ?? null);
  }, [workspace]);

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
