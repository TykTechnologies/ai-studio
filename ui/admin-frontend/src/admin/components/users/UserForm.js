import React from "react";
import { Box, Typography, CircularProgress } from "@mui/material";
import { useParams, Link } from "react-router-dom";
import {
  TitleBox,
  PrimaryButton,
  SecondaryLinkButton,
  TitleContentBox,
  DangerOutlineButton,
  StyledContentBox,
} from "../../styles/sharedStyles";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import { useUserForm } from "./hooks/useUserForm";
import { useSnackbarState } from "../../hooks/useSnackbarState";
import { ButtonContainer } from "./styles";
import UserFormBasicInfo from "./components/UserFormBasicInfo";
import UserPermissionsSection from "./components/UserPermissionsSection";
import AlertSnackbar from "../../components/common/AlertSnackbar";

const UserForm = () => {
  const { id } = useParams();
  const { snackbarState, showSnackbar, hideSnackbar } = useSnackbarState();
  
  const {
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
    submitting
  } = useUserForm(id, showSnackbar);

  if (loading) return <CircularProgress />;

  return (
    <>
      <TitleBox>
        <TitleContentBox>
          <SecondaryLinkButton
            component={Link}
            to="/admin/users"
            color="inherit"
            sx={{ mb: 1, px: 0 }}
            startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}
          >
            back to users
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">
            {id ? "Edit user" : "Create user"}
          </Typography>
        </TitleContentBox>
      </TitleBox>
      <StyledContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <UserFormBasicInfo
            name={name}
            setName={setName}
            email={email}
            setEmail={setEmail}
            password={password}
            setPassword={setPassword}
            errors={errors}
            emailVerified={emailVerified}
            setEmailVerified={setEmailVerified}
          />
          
          <UserPermissionsSection
            isAdmin={isAdmin}
            setIsAdmin={setIsAdmin}
            showPortal={showPortal}
            setShowPortal={setShowPortal}
            showChat={showChat}
            setShowChat={setShowChat}
            notificationsEnabled={notificationsEnabled}
            setNotificationsEnabled={setNotificationsEnabled}
            accessToSSOConfig={accessToSSOConfig}
            setAccessToSSOConfig={setAccessToSSOConfig}
          />

          <ButtonContainer>
            <PrimaryButton type="submit" disabled={!isFormValid() || submitting}>
              {submitting ? (
                <CircularProgress size={24} color="inherit" />
              ) : (
                id ? "Update user" : "Save user"
              )}
            </PrimaryButton>
            {id && (
              <DangerOutlineButton
                //onClick={handleDeleteClick}
              >
                Delete user
              </DangerOutlineButton>
            )}
          </ButtonContainer>
        </Box>
      </StyledContentBox>
      
      <AlertSnackbar
        open={snackbarState.open}
        message={snackbarState.message}
        severity={snackbarState.severity}
        onClose={hideSnackbar}
      />
    </>
  );
};

export default UserForm;
