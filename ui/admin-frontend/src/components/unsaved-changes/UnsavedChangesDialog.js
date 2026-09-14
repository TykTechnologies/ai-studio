import React from "react";
import ConfirmationDialog from "../../admin/components/common/ConfirmationDialog";

/**
 * The prompt shown when a navigation would discard unsaved form changes.
 * Wording is fixed here so every form gets the same dialog.
 */
const UnsavedChangesDialog = ({ open, onStay, onLeave }) => (
  <ConfirmationDialog
    open={open}
    title="Discard unsaved changes?"
    message="You have changes on this page that have not been saved. If you leave now they will be lost."
    confirmText=""
    cancelLabel="Stay"
    buttonLabel="Leave without saving"
    onConfirm={onLeave}
    onCancel={onStay}
    iconName="triangle-exclamation"
    iconColor="text.criticalDefault"
    titleColor="text.criticalDefault"
    backgroundColor="background.surfaceCriticalDefault"
    borderColor="border.criticalDefaultSubdue"
    primaryButtonComponent="danger"
    data-testid="unsaved-changes-dialog"
  />
);

export default UnsavedChangesDialog;
