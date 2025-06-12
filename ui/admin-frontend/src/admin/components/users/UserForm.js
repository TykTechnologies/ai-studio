import { Box, Typography, CircularProgress } from "@mui/material";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useEffect } from "react";
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
import useUserEntitlements from "../../hooks/useUserEntitlements";
import { ButtonContainer } from "./styles";
import UserFormBasicInfo from "./components/UserFormBasicInfo";
import UserPermissionsSection from "./components/UserPermissionsSection";
import ManageTeamsSection from "./components/ManageTeamsSection";
import AlertSnackbar from "../../components/common/AlertSnackbar";
import ConfirmationDialog from "../../components/common/ConfirmationDialog";
import { IsAdminRole } from "./utils/userRolesConfig";

const UserForm = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { snackbarState, showSnackbar, hideSnackbar } = useSnackbarState();
  const { isSuperAdmin, fetchUserEntitlements } = useUserEntitlements(true);
  
  useEffect(() => {
    fetchUserEntitlements(true);
  }, [fetchUserEntitlements]);
  
  const {
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
  } = useUserForm(id, showSnackbar);

  useEffect(() => {
    if (id && !loading && IsAdminRole(selectedRole) && !isSuperAdmin) {
      navigate(`/admin/users/${id}`);
    }
  }, [id, loading, selectedRole, isSuperAdmin, navigate]);

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
            emailVerified={emailVerified}
            setEmailVerified={setEmailVerified}
            setBasicInfoValid={setBasicInfoValid}
          />
          
          <UserPermissionsSection
            isSuperAdmin={isSuperAdmin}
            notificationsEnabled={notificationsEnabled}
            setNotificationsEnabled={setNotificationsEnabled}
            accessToSSOConfig={accessToSSOConfig}
            setAccessToSSOConfig={setAccessToSSOConfig}
            selectedRole={selectedRole}
            setSelectedRole={setSelectedRole}
          />

          <ManageTeamsSection
            selectedTeams={selectedTeams}
            setSelectedTeams={setSelectedTeams}
          />

          <ButtonContainer>
            <PrimaryButton type="submit" disabled={!basicInfoValid}>
              {id ? "Update user" : "Save user"}
            </PrimaryButton>
            {id && !isSuperAdmin && (
              <DangerOutlineButton
                onClick={handleDeleteClick}
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

      <ConfirmationDialog
        open={warningDialogOpen}
        title="Delete User"
        message="This will delete all records of this user, and they will no longer have access to Tyk AI Studio."
        buttonLabel="Delete user"
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />
    </>
  );
};

export default UserForm;
