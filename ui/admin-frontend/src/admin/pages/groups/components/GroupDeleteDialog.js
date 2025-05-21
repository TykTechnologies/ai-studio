import React from "react";
import ConfirmationDialog from "../../../components/common/ConfirmationDialog";

const GroupDeleteDialog = ({ open, selectedGroup, onConfirm, onCancel }) => {
  return (
    <ConfirmationDialog
      open={open}
      title="Delete Team"
      message={selectedGroup ? `Deleting team "${selectedGroup.attributes.name}" will remove all users from it.` : "Deleting this team will remove all users from it."}
      buttonLabel="Delete team"
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

export default GroupDeleteDialog;