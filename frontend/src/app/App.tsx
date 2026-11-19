import { QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { BrowserRouter } from "react-router-dom";

import { AppRoutes } from "../routes/AppRoutes";
import { createQueryClient } from "./queryClient";
import { WorkspaceProvider } from "./WorkspaceProvider";

/**
 * Composition root.
 *
 * The query client is outermost because the workspace provider clears the
 * cache when the tenant changes, so it must be able to reach it.
 */
export function App() {
  // Held in state so the client survives re-renders without being shared
  // across application instances.
  const [queryClient] = useState(createQueryClient);

  return (
    <QueryClientProvider client={queryClient}>
      <WorkspaceProvider>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </WorkspaceProvider>
    </QueryClientProvider>
  );
}
