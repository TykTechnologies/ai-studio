import { useState, useCallback, useEffect, useMemo } from "react";
import { runBulkAction, summariseBulkResult } from "../components/common/bulkActions";
import { defaultRowLabel } from "../components/common/DataTable";

/**
 * Selection + bulk actions for a list page.
 *
 * Selection is page-local: ids that leave the current page (paging, search,
 * a refresh after a delete) are dropped. `run(action, items)` performs the
 * action, reports one summary through `notify`, keeps the per-item failures
 * for a BulkResultAlert, clears the selection and calls `refresh`.
 * `requestDelete(items)` stages a delete behind BulkDeleteConfirmationDialog.
 *
 * @param {object} options
 * @param {Array} options.items - the rows currently on the page
 * @param {string} [options.rowKey="id"]
 * @param {string} options.resource - API collection, e.g. "llms"
 * @param {string} options.singular - noun, e.g. "LLM provider"
 * @param {string} options.plural - e.g. "LLM providers"
 * @param {(item) => string} [options.nameOf]
 * @param {(message, severity) => void} [options.notify]
 * @param {() => void} [options.refresh]
 * @param {boolean} [options.viaBulkEndpoint]
 * @param {(id) => Promise} [options.deleteOne]
 */
const useBulkActions = ({
  items,
  rowKey = "id",
  resource,
  singular,
  plural,
  nameOf = defaultRowLabel,
  notify,
  refresh,
  viaBulkEndpoint,
  deleteOne,
}) => {
  const rows = useMemo(() => (Array.isArray(items) ? items : []), [items]);
  const keyOf = useCallback((item) => String(item?.[rowKey]), [rowKey]);

  const [selectedIds, setSelectedIds] = useState([]);
  const [failures, setFailures] = useState(null);
  const [pendingDelete, setPendingDelete] = useState([]);
  const [running, setRunning] = useState(false);

  // Drop ids that are no longer on the page.
  useEffect(() => {
    setSelectedIds((previous) => {
      if (previous.length === 0) return previous;
      const present = new Set(rows.map(keyOf));
      const next = previous.filter((id) => present.has(String(id)));
      return next.length === previous.length ? previous : next;
    });
  }, [rows, keyOf]);

  const selectedItems = useMemo(() => {
    const selected = new Set(selectedIds.map((id) => String(id)));
    return rows.filter((item) => selected.has(keyOf(item)));
  }, [rows, selectedIds, keyOf]);

  const clearSelection = useCallback(() => setSelectedIds([]), []);
  const clearFailures = useCallback(() => setFailures(null), []);

  const run = useCallback(
    async (action, targets) => {
      const list = Array.isArray(targets) ? targets : [];
      if (list.length === 0) return null;
      setRunning(true);
      let summary = null;
      try {
        const result = await runBulkAction({
          resource,
          action,
          ids: list.map((item) => item?.[rowKey]),
          viaBulkEndpoint,
          deleteOne,
        });
        summary = summariseBulkResult(result, {
          singular,
          plural,
          total: list.length,
          nameOf: (id) => nameOf(list.find((item) => String(item?.[rowKey]) === String(id))),
        });
        notify?.(summary.message, summary.severity);
        setFailures(summary.failures.length > 0 ? { action, failures: summary.failures } : null);
      } catch (error) {
        console.error(`Bulk ${action} on ${resource} failed`, error);
        notify?.(`Failed to ${action} ${list.length === 1 ? singular : plural}`, "error");
      } finally {
        setRunning(false);
        setSelectedIds([]);
        refresh?.();
      }
      return summary;
    },
    [resource, rowKey, viaBulkEndpoint, deleteOne, singular, plural, nameOf, notify, refresh],
  );

  const requestDelete = useCallback((targets) => {
    setPendingDelete(Array.isArray(targets) ? targets : []);
  }, []);

  const cancelDelete = useCallback(() => setPendingDelete([]), []);

  const confirmDelete = useCallback(async () => {
    const targets = pendingDelete;
    setPendingDelete([]);
    return run("delete", targets);
  }, [pendingDelete, run]);

  const deleteDialogItems = useMemo(
    () => pendingDelete.map((item) => ({ id: item?.[rowKey], name: nameOf(item) })),
    [pendingDelete, rowKey, nameOf],
  );

  const selectionProps = useMemo(
    () => ({ selectable: true, selectedIds, onSelectionChange: setSelectedIds, rowKey }),
    [selectedIds, rowKey],
  );

  return {
    selectedIds,
    setSelectedIds,
    selectedItems,
    clearSelection,
    selectionProps,
    run,
    running,
    failures,
    clearFailures,
    requestDelete,
    confirmDelete,
    cancelDelete,
    deleteDialogItems,
    deleteDialogOpen: pendingDelete.length > 0,
  };
};

/**
 * The standard bulk action set: Activate / Deactivate (when the resource
 * supports them) and Delete (always, behind the confirmation).
 */
export const standardBulkActions = ({ run, requestDelete, canToggle = false, extra = [] }) => [
  ...(canToggle
    ? [
        { key: "activate", label: "Activate", onClick: (items) => run("activate", items) },
        { key: "deactivate", label: "Deactivate", onClick: (items) => run("deactivate", items) },
      ]
    : []),
  ...extra,
  { key: "delete", label: "Delete", danger: true, onClick: (items) => requestDelete(items) },
];

export default useBulkActions;
