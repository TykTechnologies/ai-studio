import { useState, useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { createUser, updateUser, getUser } from "../../../services/userService";
import { handleApiError } from "../../../services/utils/errorHandler";

export const useUserForm = (id, showSnackbar) => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [showPortal, setShowPortal] = useState(true);
  const [showChat, setShowChat] = useState(true);
  const [emailVerified, setEmailVerified] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [accessToSSOConfig, setAccessToSSOConfig] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState({});
  const navigate = useNavigate();

  const fetchUser = useCallback(async () => {
    if (!id) return;

    try {
      setLoading(true);
      const response = await getUser(id);
      const userData = response.data;
      
      setName(userData.attributes.name);
      setEmail(userData.attributes.email);
      setIsAdmin(userData.attributes.is_admin);
      setShowPortal(userData.attributes.show_portal ?? true);
      setShowChat(userData.attributes.show_chat ?? true);
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

  const validateForm = useCallback(() => {
    const newErrors = {};
    if (!name.trim()) newErrors.name = "Name is required";
    if (!email.trim()) newErrors.email = "Email is required";
    if (!id && !password.trim()) newErrors.password = "Password is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  }, [name, email, password, id]);

  const isFormValid = useCallback(() => {
    return (
      name.trim() !== "" &&
      email.trim() !== "" &&
      (id || password.trim() !== "")
    );
  }, [name, email, password, id]);


  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    if (!validateForm() || !isFormValid()) return;
    
    try {
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
      console.error("Error saving user", error);
      const apiError = handleApiError(error);
      showSnackbar(apiError.message, "error");
    } 
  }, [
    id,
    name,
    email,
    password,
    isAdmin,
    showPortal,
    showChat,
    emailVerified,
    notificationsEnabled,
    accessToSSOConfig,
    validateForm,
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
    isAdmin,
    setIsAdmin,
    showPortal,
    setShowPortal,
    showChat,
    setShowChat,
    emailVerified,
    setEmailVerified,
    notificationsEnabled,
    setNotificationsEnabled,
    accessToSSOConfig,
    setAccessToSSOConfig,
    loading,
    errors,
    handleSubmit,
    isFormValid,
  }), [
    name,
    email,
    password,
    isAdmin,
    showPortal,
    showChat,
    emailVerified,
    notificationsEnabled,
    accessToSSOConfig,
    loading,
    errors,
    handleSubmit,
    isFormValid,
  ]);
};