import React, { useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { Typography, CircularProgress, Box } from "@mui/material";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  TitleContentBox,
  PrimaryButton
} from "../../styles/sharedStyles";

import { useGroupForm } from "./hooks/useGroupForm";
import { useUserSelection } from "./hooks/useUserSelection";
import { useCatalogsSelection } from "./hooks/useCatalogsSelection";

import GroupFormBasicInfo from "./components/GroupFormBasicInfo";
import GroupMembersSection from "./components/GroupMembersSection";
import GroupCatalogsSection from "./components/GroupCatalogsSection";

const GroupForm = () => {
  const { id } = useParams();

  const {
    name,
    setName,
    loading: formLoading,
    error,
    selectedUsers,
    setSelectedUsers,
    selectedCatalogs,
    setSelectedCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    handleSubmit
  } = useGroupForm(id, []);


  const {
    availableUsers,
    currentPage,
    totalPages,
    isLoadingMore,
    loading: usersLoading,
    fetchUsers,
    handleUsersChange,
    handleLoadMore,
    handleSearch
  } = useUserSelection(selectedUsers, setSelectedUsers);

  const {
    catalogs,
    dataCatalogs,
    toolCatalogs,
    loading: catalogsLoading
  } = useCatalogsSelection(selectedCatalogs, selectedDataCatalogs, selectedToolCatalogs);

  
  const handleCatalogsChange = useCallback((newSelectedCatalogs) => {
    setSelectedCatalogs(newSelectedCatalogs);
  }, [setSelectedCatalogs]);
  
  const handleDataCatalogsChange = useCallback((newSelectedDataCatalogs) => {
    setSelectedDataCatalogs(newSelectedDataCatalogs);
  }, [setSelectedDataCatalogs]);
  
  const handleToolCatalogsChange = useCallback((newSelectedToolCatalogs) => {
    setSelectedToolCatalogs(newSelectedToolCatalogs);
  }, [setSelectedToolCatalogs]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  if (formLoading || usersLoading || catalogsLoading) return <CircularProgress />;

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
          md: '85%',
          lg: '75%'
        }
      }}>
        <form onSubmit={handleSubmit}>
          <GroupFormBasicInfo
            name={name}
            setName={setName}
            error={error}
          />

          <GroupMembersSection
            availableUsers={availableUsers}
            selectedUsers={selectedUsers}
            handleUsersChange={handleUsersChange}
            handleSearch={handleSearch}
            handleLoadMore={handleLoadMore}
            currentPage={currentPage}
            totalPages={totalPages}
            isLoadingMore={isLoadingMore}
          />

          <GroupCatalogsSection
            catalogs={catalogs}
            selectedCatalogs={selectedCatalogs}
            onCatalogsChange={handleCatalogsChange}
            dataCatalogs={dataCatalogs}
            selectedDataCatalogs={selectedDataCatalogs}
            onDataCatalogsChange={handleDataCatalogsChange}
            toolCatalogs={toolCatalogs}
            selectedToolCatalogs={selectedToolCatalogs}
            onToolCatalogsChange={handleToolCatalogsChange}
            loading={catalogsLoading}
          />

          <Box sx={{ display: "flex", justifyContent: "flex-start", mt: 3 }}>
            <PrimaryButton type="submit" disabled={formLoading || !name.trim()}>
              {id ? "Save team" : "Create team"}
            </PrimaryButton>
          </Box>
        </form>
      </ContentBox>
    </>
  );
};

export default GroupForm;
