import React from "react";
import { useParams, Link } from "react-router-dom";
import { Typography, CircularProgress, Box, Snackbar, Alert } from "@mui/material";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  TitleContentBox,
  PrimaryButton,
  DangerOutlineButton
} from "../../styles/sharedStyles";
import ConfirmationDialog from "../../components/common/ConfirmationDialog";

import { useGroupForm } from "./hooks/useGroupForm";
import { useCatalogsSelection } from "./hooks/useCatalogsSelection";
import useSystemFeatures from "../../hooks/useSystemFeatures";
import { getFeatureFlags } from "../../utils/featureUtils";

import GroupFormBasicInfo from "./components/GroupFormBasicInfo";
import GroupMembersSection from "./components/GroupMembersSection";
import GroupCatalogsSection from "./components/GroupCatalogsSection";

const GroupForm = () => {
  const { id } = useParams();
  const { features } = useSystemFeatures();

  const {
    name,
    setName,
    loading: formLoading,
    setSelectedUsers,
    selectedCatalogs,
    setSelectedCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    handleSubmit,
    snackbar,
    handleCloseSnackbar,
    warningDialogOpen,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  } = useGroupForm(id, []);

  const {
    catalogs,
    dataCatalogs,
    toolCatalogs,
    loading: catalogsLoading
  } = useCatalogsSelection(
    selectedCatalogs,
    selectedDataCatalogs,
    selectedToolCatalogs,
    features
  );

  const { isGatewayOnly } = getFeatureFlags(features);

  if (formLoading || catalogsLoading) return <CircularProgress />;

  return (
    <>
      <TitleBox>
        <TitleContentBox>
          <SecondaryLinkButton
            component={Link}
            to="/admin/groups"
            color="inherit"
            sx={{ mb: 1, px: 0 }}
            startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}
          >
            back to teams
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">
            {id ? "Edit team" : "Create team"}
          </Typography>
        </TitleContentBox>
      </TitleBox>

      <ContentBox sx={{
        maxWidth: {
          xs: '100%',
          sm: '100%',
          md: '100%',
          lg: '75%'
        }
      }}>
        <form onSubmit={handleSubmit}>
          <GroupFormBasicInfo
            name={name}
            setName={setName}
          />

          <GroupMembersSection
            groupId={id}
            onSelectedUsersChange={setSelectedUsers}
          />

          {!isGatewayOnly && (
            <GroupCatalogsSection
              catalogs={catalogs}
              selectedCatalogs={selectedCatalogs}
              onCatalogsChange={setSelectedCatalogs}
              dataCatalogs={dataCatalogs}
              selectedDataCatalogs={selectedDataCatalogs}
              onDataCatalogsChange={setSelectedDataCatalogs}
              toolCatalogs={toolCatalogs}
              selectedToolCatalogs={selectedToolCatalogs}
              onToolCatalogsChange={setSelectedToolCatalogs}
              loading={catalogsLoading}
              features={features}
            />
          )}

          <Box sx={{ display: "flex", justifyContent: "flex-start", mt: 3, gap: 2 }}>
            <PrimaryButton type="submit" disabled={formLoading || !name.trim()}>
              {id ? "Update team" : "Create team"}
            </PrimaryButton>
            {id && (
              <DangerOutlineButton
                onClick={handleDeleteClick}
              >
                Delete team
              </DangerOutlineButton>
            )}
          </Box>
        </form>
      </ContentBox>
      
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>

      <ConfirmationDialog
        open={warningDialogOpen}
        title="Delete Team"
        message="Deleting this team will remove all users from it."
        buttonLabel="Delete team"
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

export default GroupForm;
