import { Navigate, Route, Routes } from "react-router-dom";

import { AppLayout } from "../layouts/AppLayout";
import { BuyerPage } from "../pages/BuyerPage";
import { NotFoundPage } from "../pages/NotFoundPage";
import { SalesRequestsPage } from "../pages/SalesRequestsPage";
import { paths } from "./paths";

/** Route table for the application. */
export function AppRoutes() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route path={paths.buyer} element={<BuyerPage />} />
        <Route path="/sales" element={<Navigate to={paths.salesRequests} replace />} />
        <Route path={paths.salesRequests} element={<SalesRequestsPage />} />
        <Route path={paths.salesRequestDetail} element={<SalesRequestsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
