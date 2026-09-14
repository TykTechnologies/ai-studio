import { useState, useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";
import { handleApiError } from "../../../services/utils/errorHandler";
import { CACHE_KEYS } from "../../../utils/constants";
import { syncSubjectRoles } from "../../../services/rbacService";
import { getIdentity } from "../../../utils/identityStore";
import {
  useUnsavedForm,
  useConfirmNavigation,
  deepEqual,
} from "../../../../components/unsaved-changes";

const sortedIds = (items, key = "id") =>
  (items || []).map((item) => String(item[key])).sort();

export const useGroupForm = (id, initialCatalogs = [], initialDataCatalogs = [], initialToolCatalogs = []) => {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  // True once an existing team is on screen; the unsaved-changes baseline is
  // taken then (at once for a new team).
  const [loaded, setLoaded] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  // The membership as stored, so the picker's late-arriving initial report
  // (GroupMembersSection fetches it on its own) is not read as an edit.
  const [initialMemberIds, setInitialMemberIds] = useState([]);
  
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  // Roles bound to the team (Enterprise); ids only.
  const [selectedRoleIds, setSelectedRoleIds] = useState([]);
  // Plugin resource instances per type, as reported by
  // GroupPluginResourcesSection: { "<pluginId>:<slug>": [instanceId] }.
  // Stays null until the section has loaded, so a save never wipes
  // assignments the user could not see.
  const [pluginResourceSelections, setPluginResourceSelectionsState] = useState(null);
  // The section's first report is what the server holds; only a later
  // difference from it is a user change.
  const [initialPluginResourceSelections, setInitialPluginResourceSelections] = useState(null);
  const setPluginResourceSelections = useCallback((selections) => {
    setPluginResourceSelectionsState(selections);
    setInitialPluginResourceSelections((prev) => (prev === null ? selections : prev));
  }, []);

  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);

  const navigate = useNavigate();

  // Unsaved-changes tracking. Members and plugin resources arrive from their
  // sections after mount, so each is folded in as "differs from what was
  // loaded" rather than as a raw value the baseline could catch half-loaded:
  // the members list only counts once the picker has caught up with the
  // stored membership.
  const currentMemberIds = sortedIds(selectedUsers);
  const [membersSynced, setMembersSynced] = useState(!id);
  useEffect(() => {
    if (!membersSynced && loaded && deepEqual(currentMemberIds, initialMemberIds)) {
      setMembersSynced(true);
    }
  }, [membersSynced, loaded, currentMemberIds, initialMemberIds]);
  const { markSaved } = useUnsavedForm(
    {
      name,
      memberIds: membersSynced ? currentMemberIds : initialMemberIds,
      catalogIds: sortedIds(selectedCatalogs, "value"),
      dataCatalogIds: sortedIds(selectedDataCatalogs, "value"),
      toolCatalogIds: sortedIds(selectedToolCatalogs, "value"),
      roleIds: [...selectedRoleIds].map(String).sort(),
      pluginResourcesChanged:
        initialPluginResourceSelections !== null &&
        !deepEqual(initialPluginResourceSelections, pluginResourceSelections),
    },
    { ready: !id || loaded }
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = useCallback(
    () => confirmNavigation(() => navigate("/admin/groups")),
    [confirmNavigation, navigate]
  );

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

      // Stored membership, for the unsaved-changes baseline (the members
      // section loads the same list for display).
      try {
        const members = await teamsService.getTeamUsers(id, { all: true });
        setInitialMemberIds(sortedIds(members?.data || []));
      } catch (membersError) {
        console.error("Error fetching team members", membersError);
      }

      setLoaded(true);
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

      // Plugin resources live behind their own endpoint; only send them when
      // the section reported a selection (see pluginResourceSelections).
      if (pluginResourceSelections && groupId) {
        await teamsService.updateGroupPluginResources(groupId, pluginResourceSelections);
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
      markSaved();
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
  }, [id, name, selectedUsers, selectedCatalogs, selectedDataCatalogs, selectedToolCatalogs, selectedRoleIds, pluginResourceSelections, navigate, markSaved]);

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
      // The team is gone; nothing typed can be saved any more.
      markSaved();
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
  }, [id, navigate, markSaved]);

  return useMemo(() => ({
    name,
    setName,
    loading,
    handleCancel,
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
    pluginResourceSelections,
    setPluginResourceSelections,
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
    handleCancel,
    setPluginResourceSelections,
    selectedUsers,
    selectedCatalogs,
    selectedDataCatalogs,
    selectedToolCatalogs,
    selectedRoleIds,
    pluginResourceSelections,
    snackbar,
    warningDialogOpen,
    handleSubmit,
    handleCloseSnackbar,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  ]);
};