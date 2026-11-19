import { NavLink, Outlet } from "react-router-dom";

import { Alert, WorkspaceSelector } from "../components/common";
import { useWorkspace } from "../hooks/useWorkspace";
import { paths } from "../routes/paths";

/**
 * The application shell: brand, primary navigation, workspace selector and
 * the routed page.
 *
 * Nothing is fetched before a workspace is chosen, so the shell says what to
 * do rather than letting pages fail with a missing-header error.
 */
export function AppLayout() {
  const { workspace } = useWorkspace();

  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="app-brand">
          <span className="app-brand__mark">BeautyScale</span>
          <span className="app-brand__subtitle">Brand order requests</span>
        </div>

        <nav className="app-nav" aria-label="Primary">
          <NavLink to={paths.buyer} end className={navLinkClass}>
            Buyer
          </NavLink>
          <NavLink to={paths.salesRequests} className={navLinkClass}>
            Sales
          </NavLink>
        </nav>

        <WorkspaceSelector />
      </header>

      <main className="app-main">
        {workspace ? (
          <Outlet />
        ) : (
          <Alert tone="warning" title="Choose a workspace">
            Every request is scoped to one workspace. Pick one above to continue —
            run <code>make seed</code> in the backend to create the demo workspaces
            and print their IDs.
          </Alert>
        )}
      </main>
    </div>
  );
}

function navLinkClass({ isActive }: { isActive: boolean }): string {
  return isActive ? "app-nav__link app-nav__link--active" : "app-nav__link";
}
