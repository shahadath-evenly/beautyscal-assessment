import { createContext } from "react";

import type { Workspace } from "../types/workspace";

export interface WorkspaceContextValue {
  /** The workspace every request is sent as, or null when none is chosen. */
  workspace: Workspace | null;
  /** The configured demo workspaces. */
  workspaces: Workspace[];
  /** Selects a workspace, persisting the choice for this browser. */
  selectWorkspace: (workspace: Workspace) => void;
  /** Selects a workspace by a pasted ID, for when none are configured. */
  selectWorkspaceById: (id: string, name?: string) => void;
  /** Clears the selection. */
  clearWorkspace: () => void;
}

/**
 * Undefined by default, so `useWorkspace` can tell "no provider" from
 * "no workspace chosen" and fail loudly on the first.
 */
export const WorkspaceContext = createContext<WorkspaceContextValue | undefined>(
  undefined,
);
