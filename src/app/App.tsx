import { QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { BrowserRouter } from "react-router-dom";

import { AppRoutes } from "../routes/AppRoutes";
import { createQueryClient } from "./queryClient";

/**
 * Composition root: the query client and router are provided here, so pages
 * and features receive everything they need through context and props.
 */
export function App() {
  // Held in state so the client survives re-renders but is never shared
  // across application instances.
  const [queryClient] = useState(createQueryClient);

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
