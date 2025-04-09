import React, { useState, useEffect, useCallback } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  Snackbar,
} from "@mui/material";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import EditIcon from "@mui/icons-material/Edit";
import apiClient from "../../utils/apiClient";
import { copyToClipboard } from "../../utils/clipboardUtils";
import { mapApiToUIProfile } from "./UIProfile";
import CollapsibleSection from "../common/CollapsibleSection";
import ProfileDetailsSection from "./ProfileDetailsSection";
import ProviderConfigurationSection from "./ProviderConfigurationSection";
import UserGroupMappingSection from "./UserGroupMappingSection";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryLinkButton,
} from "../../styles/sharedStyles";

const SSOProfileDetails = () => {
  const { profileId } = useParams();
  const navigate = useNavigate();
  const [profileData, setProfileData] = useState(null);
  const [profileMetadata, setProfileMetadata] = useState({
    loginUrl: "",
    callbackUrl: "",
    failureRedirectUrl: "",
    selectedProviderType: "",
    useInLoginPage: false,
  });
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [groupsError, setGroupsError] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  // Handle copy to clipboard
  const handleCopyToClipboard = async (text, fieldName) => {
    await copyToClipboard(text, fieldName,
      (field) => {
        setSnackbar({
          open: true,
          message: `${field} copied to clipboard`,
          severity: "success",
        });
      },
      (field) => {
        setSnackbar({
          open: true,
          message: `Failed to copy ${field}`,
          severity: "error",
        });
      }
    );
  };

  // Fetch profile data
  const fetchProfileData = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/sso-profiles/${profileId}`);
      const uiProfile = mapApiToUIProfile(response.data);
      setProfileData(uiProfile);
      
      // Extract metadata from API response
      setProfileMetadata({
        loginUrl: response.data.data.attributes.login_url || "",
        callbackUrl: response.data.data.attributes.callback_url || "",
        failureRedirectUrl: response.data.data.attributes.failure_redirect_url || "",
        selectedProviderType: response.data.data.attributes.selected_provider_type || "",
        useInLoginPage: response.data.data.attributes.use_in_login_page || false,
      });
      
      setError("");
    } catch (error) {
      console.error("Error fetching Identity provider profile", error);
      setError("Failed to load Identity provider profile");
    } finally {
      setLoading(false);
    }
  }, [profileId]);

  // Fetch all groups
  const fetchGroups = useCallback(async () => {
    try {
      const response = await apiClient.get("/groups", {
        params: { all: true },
      });
      setGroups(response.data.data || []);
      setGroupsError("");
    } catch (error) {
      console.error("Error fetching groups", error);
      setGroupsError("Failed to load groups. Group names may not be displayed correctly.");
    }
  }, []);

  useEffect(() => {
    fetchProfileData();
    fetchGroups();
  }, [fetchProfileData, fetchGroups]);

  const handleEditProfile = () => {
    navigate(`/admin/sso-profiles/edit/${profileId}`);
  };

  // Helper function to get group name by ID
  const getGroupNameById = (groupId) => {
    const group = groups.find((g) => g.id === groupId);
    return group ? group.attributes.name : groupId;
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">{error}</Alert>
      </Box>
    );
  }

  if (!profileData) {
    return null;
  }

  return (
    <Box>
      <TitleBox sx={{ display: 'flex', alignItems: 'flex-end' }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
          <SecondaryLinkButton
            component={Link}
            to="/admin/sso-profiles"
            color="inherit"
            sx={{ mb: 1, px: 0 }}
            startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}
          >
            back to IdP profiles
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">
            Profile - {profileData.Name || profileData.ID}
          </Typography>
        </Box>
        <Box>
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={handleEditProfile}
          >
            Edit profile
          </PrimaryButton>
        </Box>
      </TitleBox>

      <ContentBox>
        {/* Profile Details Section */}
        <CollapsibleSection title="Profile details">
          <ProfileDetailsSection
            profileData={profileData}
            profileMetadata={profileMetadata}
            handleCopyToClipboard={handleCopyToClipboard}
          />
        </CollapsibleSection>

        {/* Provider Configuration Section */}
        <CollapsibleSection title="Provider configuration">
          <ProviderConfigurationSection
            profileData={profileData}
            profileMetadata={profileMetadata}
            handleCopyToClipboard={handleCopyToClipboard}
          />
        </CollapsibleSection>

        {/* User Group Mapping Section */}
        <CollapsibleSection title="User group mapping">
          <UserGroupMappingSection 
            profileData={profileData} 
            groups={groups} 
            groupsError={groupsError} 
            getGroupNameById={getGroupNameById} 
          />
        </CollapsibleSection>
      </ContentBox>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default SSOProfileDetails;