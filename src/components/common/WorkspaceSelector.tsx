import { useState } from "react";

import { useWorkspace } from "../../hooks/useWorkspace";
import { workspacesService } from "../../service";
import { Button } from "./Button";
import { Input } from "./Input";
import { Select } from "./Select";

/**
 * Chooses which workspace the app acts as.
 *
 * This is a demo convenience, not authentication: the backend trusts the
 * X-Workspace-ID header it validates, and this only decides what is sent. The
 * label says so, so nobody mistakes it for a login.
 *
 * When no workspaces are configured through VITE_WORKSPACES, it accepts a
 * pasted ID instead, which is what the seed command prints.
 */
export function WorkspaceSelector() {
  const { workspace, workspaces, selectWorkspace, selectWorkspaceById } = useWorkspace();
  const [manualId, setManualId] = useState("");
  const [touched, setTouched] = useState(false);

  if (workspaces.length > 0) {
    return (
      <label className="workspace-selector">
        <span className="workspace-selector__label">
          Workspace
          <span className="workspace-selector__hint"> · demo selector, not a login</span>
        </span>
        <Select
          value={workspace?.id ?? ""}
          placeholder="Choose a workspace"
          aria-label="Active workspace (demo selector, not authentication)"
          options={workspaces.map((option) => ({ value: option.id, label: option.name }))}
          onChange={(event) => {
            const next = workspaces.find((option) => option.id === event.target.value);
            if (next) selectWorkspace(next);
          }}
        />
      </label>
    );
  }

  const trimmed = manualId.trim();
  const invalid = touched && trimmed !== "" && !workspacesService.isValidId(trimmed);

  return (
    <form
      className="workspace-selector workspace-selector--manual"
      onSubmit={(event) => {
        event.preventDefault();
        setTouched(true);
        if (workspacesService.isValidId(trimmed)) {
          selectWorkspaceById(trimmed);
          setManualId("");
          setTouched(false);
        }
      }}
    >
      <label className="workspace-selector__label" htmlFor="workspace-id">
        Workspace ID
        <span className="workspace-selector__hint"> · from `make seed`</span>
      </label>
      <div className="workspace-selector__manual-row">
        <Input
          id="workspace-id"
          value={manualId}
          invalid={invalid}
          placeholder={workspace?.id ?? "00000000-0000-0000-0000-000000000000"}
          onChange={(event) => setManualId(event.target.value)}
          onBlur={() => setTouched(true)}
        />
        <Button type="submit" variant="primary" size="sm">
          Use
        </Button>
      </div>
      {invalid && (
        <p className="field__error" role="alert">
          That does not look like a workspace ID.
        </p>
      )}
    </form>
  );
}
