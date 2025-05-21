import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";

const useGroupActions = (refreshGroups, setSnackbar) => {
  const navigate = useNavigate();
  const [selectedGroup, setSelectedGroup] = useState(null);
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);

  const handleEdit = (group) => {
    if (group) {
      navigate(`/admin/groups/edit/${group.id}`);
    } else if (selectedGroup) {
      navigate(`/admin/groups/edit/${selectedGroup.id}`);
    }
  };

  const handleDelete = (group) => {
    if (group) {
      setSelectedGroup(group);
      setWarningDialogOpen(true);
    }
  };

  const handleCancelDelete = () => {
    setWarningDialogOpen(false);
    setSelectedGroup(null);
  };

  const handleConfirmDelete = async () => {
    if (selectedGroup) {
      try {
        await teamsService.deleteTeam(selectedGroup.id);
        setSnackbar({
          open: true,
          message: `Team "${selectedGroup.attributes.name}" deleted successfully!`,
          severity: "success",
        });
        refreshGroups();
      } catch (error) {
        console.error("Error deleting group", error);
        setSnackbar({
          open: true,
          message: `Failed to delete team "${selectedGroup.attributes.name}".`,
          severity: "error",
        });
      } finally {
        handleCancelDelete();
      }
    }
  };

  const handleGroupClick = (group) => {
    navigate(`/admin/groups/${group.id}`);
  };

  return {
    selectedGroup,
    warningDialogOpen,
    handleEdit,
    handleDelete,
    handleCancelDelete,
    handleConfirmDelete,
    handleGroupClick,
  };
};

export default useGroupActions;