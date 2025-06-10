import { useState, useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { createUser, updateUser, getUser } from "../../../services/userService";
import { handleApiError } from "../../../services/utils/errorHandler";

export const useUserForm = (id, showSnackbar) => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [emailVerified, setEmailVerified] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [accessToSSOConfig, setAccessToSSOConfig] = useState(false);
  const [selectedRole, setSelectedRole] = useState("Chat user");
  const [loading, setLoading] = useState(false);
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
      setEmailVerified(userData.attributes.email_verified ?? false);
      setNotificationsEnabled(userData.attributes.notifications_enabled ?? false);
      setAccessToSSOConfig(userData.attributes.access_to_sso_config ?? false);
    } catch (error) {
      const apiError = handleApiError(error);
      showSnackbar(apiError.message, "error");
    } finally {
      setLoading(false);
    }
  }, [id, showSnackbar]);


  useEffect(() => {
    if (id) {
      fetchUser();
    }
  }, [id, fetchUser]);


  const isFormValid = useCallback(() => {
    return (
      name.trim() !== "" &&
      email.trim() !== "" &&
      password.trim() !== ""
    );
  }, [name, email, password]);


  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    if (!isFormValid()) return;
    
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
      const apiError = handleApiError(error);
      showSnackbar(apiError.message, "error");
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
    isFormValid,
    navigate,
    showSnackbar,
  ]);

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
    loading,
    handleSubmit,
    isFormValid,
  }), [
    name,
    email,
    password,
    emailVerified,
    notificationsEnabled,
    accessToSSOConfig,
    selectedRole,
    loading,
    handleSubmit,
    isFormValid,
  ]);
};