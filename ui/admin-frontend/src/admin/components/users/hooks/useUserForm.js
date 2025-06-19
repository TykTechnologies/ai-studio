import { useState, useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { createUser, updateUser, getUser, deleteUser } from "../../../services/userService";

export const useUserForm = (id, showSnackbar) => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [emailVerified, setEmailVerified] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [accessToSSOConfig, setAccessToSSOConfig] = useState(false);
  const [selectedRole, setSelectedRole] = useState("Chat user");
  const [selectedTeams, setSelectedTeams] = useState([]);
  const [loading, setLoading] = useState(false);
  const [basicInfoValid, setBasicInfoValid] = useState(false);
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);
  const navigate = useNavigate();

  const fetchUser = useCallback(async () => {
    if (!id) return;

    try {
      setLoading(true);
      const response = await getUser(id);
      const userData = response.data;
      
      setName(userData.attributes.name);
      setEmail(userData.attributes.email);
      setSelectedRole(userData.attributes.role);
      if (userData.attributes.groups && userData.attributes.groups.length > 0) {
        setSelectedTeams(userData.attributes.groups.map(group => parseInt(group.id, 10)));
      }
      setEmailVerified(userData.attributes.email_verified ?? false);
      setNotificationsEnabled(userData.attributes.notifications_enabled ?? false);
      setAccessToSSOConfig(userData.attributes.access_to_sso_config ?? false);
    } catch (error) {
      showSnackbar(error.message, "error");
    } finally {
      setLoading(false);
    }
  }, [id, showSnackbar]);

  useEffect(() => {
    if (id) {
      fetchUser();
    }
  }, [id, fetchUser]);

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    if (!basicInfoValid) return;
    
    try {
      const isAdmin = selectedRole === "Admin";
      const showPortal = selectedRole === "Developer" || selectedRole === "Admin";
      const showChat = true;
      
      const userData = {
        name,
        email,
        isAdmin,
        showPortal,
        showChat,
        emailVerified,
        notificationsEnabled: isAdmin ? notificationsEnabled : false,
        accessToSSOConfig: isAdmin ? accessToSSOConfig : false,
        groups: selectedTeams,
        ...(password && { password }),
      };

      if (id) {
        await updateUser(id, userData);
        showSnackbar("User updated successfully", "success");
      } else {
        await createUser(userData);
        showSnackbar("User created successfully", "success");
      }

      setTimeout(() => navigate("/admin/users"), 2000);
    } catch (error) {
      showSnackbar(error.message, "error");
    }
  }, [
    id,
    name,
    email,
    password,
    selectedRole,
    emailVerified,
    notificationsEnabled,
    accessToSSOConfig,
    selectedTeams,
    navigate,
    showSnackbar,
    basicInfoValid,
  ]);

  const handleDeleteClick = useCallback(() => {
    setWarningDialogOpen(true);
  }, []);

  const handleCancelDelete = useCallback(() => {
    setWarningDialogOpen(false);
  }, []);

  const handleConfirmDelete = useCallback(async () => {
    try {
      await deleteUser(id);
      showSnackbar("User deleted successfully", "success");
      setTimeout(() => navigate("/admin/users"), 2000);
    } catch (error) {
      showSnackbar(error.message, "error");
    } finally {
      setWarningDialogOpen(false);
    }
  }, [id, navigate, showSnackbar]);

  return useMemo(() => ({
    name,
    setName,
    email,
    setEmail,
    password,
    setPassword,
    emailVerified,
    setEmailVerified,
    notificationsEnabled,
    setNotificationsEnabled,
    accessToSSOConfig,
    setAccessToSSOConfig,
    selectedRole,
    setSelectedRole,
    selectedTeams,
    setSelectedTeams,
    loading,
    handleSubmit,
    setBasicInfoValid,
    basicInfoValid,
    warningDialogOpen,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  }), [
    name,
    email,
    password,
    emailVerified,
    notificationsEnabled,
    accessToSSOConfig,
    selectedRole,
    selectedTeams,
    loading,
    handleSubmit,
    setBasicInfoValid,
    basicInfoValid,
    warningDialogOpen,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  ]);
};