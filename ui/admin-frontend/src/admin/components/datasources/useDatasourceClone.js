import { useCallback, useState } from "react";
import apiClient from "../../utils/apiClient";
import { P } from "../../rbac/permissions";
import { useCanMove, workspaceFieldError } from "../workspaces/workspaceFields";

/**
 * useDatasourceClone clones a data source on the server (which keeps its
 * stored credentials). The copy lands by the landing rule; a caller who can
 * write data sources in more than one Workspace first picks where it goes
 * (WorkspacePickDialog with the returned dialogProps). onCloned receives
 * the new data source's id; onError a message for anything else.
 */
const useDatasourceClone = ({ onCloned, onError }) => {
  const canChoose = useCanMove(P.DATASOURCES_WRITE);
  const [sourceId, setSourceId] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const clone = useCallback(
    async (id, workspaceId) => {
      setBusy(true);
      try {
        const body =
          workspaceId !== null && workspaceId !== undefined
            ? { data: { attributes: { workspace_id: Number(workspaceId) } } }
            : undefined;
        const response = await apiClient.post(`/datasources/${id}/clone`, body);
        setSourceId(null);
        setError("");
        onCloned(response.data.data.id);
      } catch (err) {
        console.error("Error cloning datasource:", err);
        const wsError = workspaceFieldError(err);
        if (wsError) {
          // Ask (or ask again) where the copy should go.
          setSourceId(id);
          setError(wsError);
        } else {
          setSourceId(null);
          onError("Failed to clone data source");
        }
      } finally {
        setBusy(false);
      }
    },
    [onCloned, onError],
  );

  const startClone = useCallback(
    (id) => {
      setError("");
      if (canChoose) setSourceId(id);
      else clone(id, null);
    },
    [canChoose, clone],
  );

  const dialogProps = {
    open: sourceId !== null,
    permission: P.DATASOURCES_WRITE,
    title: "Clone data source",
    description: "Choose the Workspace the copy is created in.",
    confirmLabel: "Clone",
    error,
    busy,
    onConfirm: (workspaceId) => clone(sourceId, workspaceId),
    onCancel: () => {
      setSourceId(null);
      setError("");
    },
  };

  return { startClone, dialogProps };
};

export default useDatasourceClone;
