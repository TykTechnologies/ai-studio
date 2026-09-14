import React, { useEffect, useState } from "react";
import ConfirmationDialog from "./ConfirmationDialog";
import {
  fetchDependents,
  buildDependentsMessage,
} from "../../utils/dependentsMessage";

/**
 * Delete confirmation that states the consequence and the dependents of the
 * object about to be removed. The dependents are fetched when the dialog
 * opens; if the request fails the message falls back to a generic sentence.
 *
 * @param {boolean} open
 * @param {string} resourcePath - API collection, e.g. "llms", "model-routers"
 * @param {string} objectLabel - noun used in the sentence, e.g. "LLM", "tool"
 * @param {{id: string|number, name: string}|null} item
 * @param {string} [consequence] - trailing sentence after the dependents list
 * @param {Function} onConfirm
 * @param {Function} onCancel
 */
const DeleteConfirmationDialog = ({
  open,
  resourcePath,
  objectLabel,
  item,
  consequence,
  onConfirm,
  onCancel,
}) => {
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (!open || !item?.id) {
      return undefined;
    }
    let cancelled = false;
    setMessage("Checking where this is used…");
    fetchDependents(resourcePath, item.id).then((dependents) => {
      if (!cancelled) {
        setMessage(buildDependentsMessage(dependents, objectLabel, consequence));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [open, item?.id, resourcePath, objectLabel, consequence]);

  return (
    <ConfirmationDialog
      open={open}
      title={`Delete ${item?.name || objectLabel}?`}
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
    />
  );
};

export default DeleteConfirmationDialog;
