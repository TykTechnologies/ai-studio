import { useState, useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";
import { handleApiError } from "../../../services/utils/errorHandler";
import { CACHE_KEYS } from "../../../utils/constants";
import { syncSubjectRoles } from "../../../services/rbacService";
import { getIdentity } from "../../../utils/identityStore";

export const useGroupForm = (id, initialCatalogs = [], initialDataCatalogs = [], initialToolCatalogs = []) => {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  // Roles bound to the team (Enterprise); ids only.
  const [selectedRoleIds, setSelectedRoleIds] = useState([]);

  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);
  
  const navigate = useNavigate();

  const fetchGroup = useCallback(async () => {
    if (!id) return;

    try {
      setLoading(true);
      const response = await teamsService.getTeam(id);
      setName(response.data.attributes.name);
      
      const { catalogues, data_catalogues, tool_catalogues } = response.data.attributes;
      
      if (catalogues) {
        setSelectedCatalogs(catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (data_catalogues) {
        setSelectedDataCatalogs(data_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (tool_catalogues) {
        setSelectedToolCatalogs(tool_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }

      setSelectedRoleIds((response.data.attributes.roles || []).map(r => Number(r.id)));

      setLoading(false);
    } catch (error) {
      console.error("Error fetching group", error);
      const apiError = handleApiError(error);
      setSnackbar({
        open: true,
        message: apiError.message,
        severity: "error",
      });
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    if (id) {
      fetchGroup();
    }
  }, [id, fetchGroup]);

  const handleCloseSnackbar = useCallback((_, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar(prev => ({ ...prev, open: false }));
  }, []);

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    setLoading(true);

    const groupData = {
      data: {
        type: "Group",
        attributes: {
          name,
          members: selectedUsers.map(user => parseInt(user.id, 10)),
          catalogues: selectedCatalogs.map(cat => parseInt(cat.value, 10)),
          data_catalogues: selectedDataCatalogs.map(cat => parseInt(cat.value, 10)),
          tool_catalogues: selectedToolCatalogs.map(cat => parseInt(cat.value, 10))
        },
      },
    };

    try {
      let groupId = id;
      if (id) {
        await teamsService.updateTeam(id, groupData);

        localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
          operation: "update",
          message: "Team updated successfully",
          timestamp: Date.now()
        }));
      } else {
        const created = await teamsService.createTeam(groupData);
        groupId = created?.data?.id;

        localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
          operation: "create",
          message: "Team created successfully",
          timestamp: Date.now()
        }));
      }

      // Enterprise: make the team's role bindings match the selection.
      if (getIdentity()?.rbacEnabled && groupId) {
        const { failed } = await syncSubjectRoles("group", groupId, selectedRoleIds);
        if (failed.length > 0) {
          setSnackbar({
            open: true,
            message: `Team saved, but ${failed.length} role change(s) could not be applied: ${failed.map(f => f.error?.message).filter(Boolean).join("; ")}`,
            severity: "error",
          });
          setLoading(false);
          return;
        }
      }
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error saving group", error);
      const apiError = handleApiError(error);
      setSnackbar({
        open: true,
        message: apiError.message,
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [id, name, selectedUsers, selectedCatalogs, selectedDataCatalogs, selectedToolCatalogs, selectedRoleIds, navigate]);

  const handleDeleteClick = useCallback(() => {
    setWarningDialogOpen(true);
  }, []);

  const handleCancelDelete = useCallback(() => {
    setWarningDialogOpen(false);
  }, []);

  const handleConfirmDelete = useCallback(async () => {
    try {
      setLoading(true);
      await teamsService.deleteTeam(id);
      localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
        operation: "delete",
        message: "Team deleted successfully",
        timestamp: Date.now()
      }));
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error deleting team:", error);
      const apiError = handleApiError(error);
      setSnackbar({
        open: true,
        message: apiError.message,
        severity: "error",
      });
    } finally {
      setWarningDialogOpen(false);
      setLoading(false);
    }
  }, [id, navigate]);

  return useMemo(() => ({
    name,
    setName,
    loading,
    selectedUsers,
    setSelectedUsers,
    selectedCatalogs,
    setSelectedCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    selectedRoleIds,
    setSelectedRoleIds,
    handleSubmit,
    snackbar,
    handleCloseSnackbar,
    warningDialogOpen,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  }), [
    name,
    loading,
    selectedUsers,
    selectedCatalogs,
    selectedDataCatalogs,
    selectedToolCatalogs,
    selectedRoleIds,
    snackbar,
    warningDialogOpen,
    handleSubmit,
    handleCloseSnackbar,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  ]);
};