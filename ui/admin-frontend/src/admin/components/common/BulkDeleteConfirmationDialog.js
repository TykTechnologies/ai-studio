import React, { useEffect, useMemo, useState } from "react";
import ConfirmationDialog from "./ConfirmationDialog";
import {
  DEPENDENT_GROUPS,
  fetchDependents,
  groupsForDependents,
} from "../../utils/dependentsMessage";

/**
 * Delete confirmation for a selection. Titles "Delete 3 LLM providers?" and,
 * for up to MAX_DEPENDENT_LOOKUPS items, fetches each one's dependents and
 * states the aggregated consequence ("2 of them are used by 3 apps (…) and
 * 1 catalog (…). …"). Larger selections, and resources without a dependents
 * endpoint (pass resourcePath null), get the generic sentence.
 *
 * @param {boolean} open
 * @param {string|null} resourcePath - API collection, e.g. "llms"; null skips lookups
 * @param {string} objectLabel - singular noun, e.g. "LLM provider"
 * @param {string} [objectLabelPlural] - defaults to objectLabel + "s"
 * @param {Array<{id: string|number, name: string}>} items
 * @param {string} [consequence] - trailing sentence after the dependents list
 * @param {Function} onConfirm
 * @param {Function} onCancel
 */

export const MAX_DEPENDENT_LOOKUPS = 5;
const MAX_NAMES = 3;

const joinParts = (parts) => {
  if (parts.length <= 1) return parts.join("");
  return `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;
};

/**
 * Builds the consequence sentence for several objects at once.
 * @param {Array<object|null>} dependentsList - one dependents block per item
 *   (null where the lookup failed)
 * @param {string} plural - plural noun for the objects being deleted
 * @param {string} consequence
 */
export const buildBulkDependentsMessage = (
  dependentsList,
  plural,
  consequence = "Deleting them removes them from all of those.",
) => {
  const total = dependentsList.length;
  const known = dependentsList.filter(Boolean);
  const usedCount = known.filter((dependents) => groupsForDependents(dependents).length > 0).length;

  if (usedCount === 0) {
    if (known.length < total) {
      return `Deleting these ${plural} removes them from everything that references them.`;
    }
    return `Nothing references these ${plural}.`;
  }

  // Union of dependents per group, de-duplicated by id, in display order.
  const byGroup = new Map();
  known.forEach((dependents) => {
    groupsForDependents(dependents).forEach((group) => {
      const bucket = byGroup.get(group.key) || new Map();
      group.items.forEach((item) => {
        const id = item?.id ?? item?.name;
        if (!bucket.has(id)) bucket.set(id, item);
      });
      byGroup.set(group.key, bucket);
    });
  });

  const parts = DEPENDENT_GROUPS.filter((group) => byGroup.has(group.key)).map((group) => {
    const items = Array.from(byGroup.get(group.key).values());
    const count = items.length;
    const noun = count === 1 ? group.singular : group.plural;
    const names = items.map((item) => item?.name).filter(Boolean).slice(0, MAX_NAMES);
    const overflow = count - names.length;
    const listed =
      names.length === 0 ? "" : ` (${names.join(", ")}${overflow > 0 ? `, +${overflow} more` : ""})`;
    return `${count} ${noun}${listed}`;
  });

  const subject =
    usedCount === total
      ? total === 1
        ? "It is"
        : "All of them are"
      : `${usedCount} of them ${usedCount === 1 ? "is" : "are"}`;
  const lookupNote = known.length < total ? " Some could not be checked." : "";
  return `${subject} used by ${joinParts(parts)}. ${consequence}${lookupNote}`;
};

const BulkDeleteConfirmationDialog = ({
  open,
  resourcePath,
  objectLabel,
  objectLabelPlural,
  items,
  consequence,
  onConfirm,
  onCancel,
}) => {
  const [message, setMessage] = useState("");
  const list = useMemo(() => (Array.isArray(items) ? items : []), [items]);
  const plural = objectLabelPlural || `${objectLabel}s`;
  // Re-run the lookup when the selection changes, not when the array identity does.
  const idsKey = list.map((item) => String(item.id)).join(",");

  useEffect(() => {
    if (!open || list.length === 0) {
      return undefined;
    }
    if (!resourcePath || list.length > MAX_DEPENDENT_LOOKUPS) {
      setMessage(
        list.length === 1
          ? `Deleting this ${objectLabel} removes it from everything that references it.`
          : `Deleting these ${plural} removes them from everything that references them.`,
      );
      return undefined;
    }
    let cancelled = false;
    setMessage(list.length === 1 ? "Checking where this is used…" : "Checking where these are used…");
    Promise.all(list.map((item) => fetchDependents(resourcePath, item.id))).then((dependentsList) => {
      if (!cancelled) {
        setMessage(buildBulkDependentsMessage(dependentsList, plural, consequence));
      }
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, idsKey, resourcePath, objectLabel, plural, consequence]);

  const title =
    list.length === 1
      ? `Delete ${list[0]?.name || objectLabel}?`
      : `Delete ${list.length} ${plural}?`;

  return (
    <ConfirmationDialog
      open={open}
      title={title}
      message={message}
      confirmText="This cannot be undone."
      buttonLabel="Delete"
      onConfirm={onConfirm}
      onCancel={onCancel}
      iconName="hexagon-exclamation"
      iconColor="background.buttonCritical"
      titleColor="text.criticalDefault"
      backgroundColor="background.surfaceCriticalDefault"
      borderColor="border.criticalDefaultSubdue"
      primaryButtonComponent="danger"
      data-testid="bulk-delete-dialog"
    />
  );
};

export default BulkDeleteConfirmationDialog;
