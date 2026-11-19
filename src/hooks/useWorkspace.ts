import { useContext } from "react";

import { WorkspaceContext, type WorkspaceContextValue } from "../app/WorkspaceContext";

/**
 * The active workspace and the controls for changing it.
 *
 * Throws when used outside the provider: that is a wiring mistake, and
 * failing loudly beats rendering a page that silently queries no tenant.
 */
export function useWorkspace(): WorkspaceContextValue {
  const context = useContext(WorkspaceContext);
  if (!context) {
    throw new Error("useWorkspace must be used within a WorkspaceProvider");
  }
  return context;
}

/**
 * The active workspace ID, or null.
 *
 * Features pass this into query keys and use it to gate queries, so nothing
 * is fetched before a workspace is chosen.
 */
export function useWorkspaceId(): string | null {
  return useWorkspace().workspace?.id ?? null;
}
