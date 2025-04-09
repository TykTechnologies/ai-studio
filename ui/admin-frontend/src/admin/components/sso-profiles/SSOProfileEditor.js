import React, { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import Editor from "@monaco-editor/react";
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  Snackbar,
  Paper,
} from "@mui/material";
import ConfirmationDialog from "../../components/common/ConfirmationDialog";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import apiClient from "../../utils/apiClient";
import { createEmptyProfile, mapApiToUIProfile, mapUIProfileToApi } from "./UIProfile";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryLinkButton,
  DangerOutlineButton
} from "../../styles/sharedStyles";

// Constant for localStorage key
const SSO_NOTIFICATION_KEY = 'tyk_ai_studio_admin_sso_notification';

const SSOProfileEditor = () => {
  const { profileId } = useParams();
  const navigate = useNavigate();
  const isEditMode = profileId && profileId !== "new";
  
  // Use our UI Profile model for state management
  // We store the profile data but primarily use the editor content for UI
  const [, setProfileData] = useState(createEmptyProfile());
  const [editorContent, setEditorContent] = useState(JSON.stringify(createEmptyProfile(), null, 2));
  const [loading, setLoading] = useState(isEditMode && profileId);
  const [error, setError] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);

  const fetchProfileData = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/sso-profiles/${profileId}`);
      const uiProfile = mapApiToUIProfile(response.data);
      
      setProfileData(uiProfile);
      setEditorContent(JSON.stringify(uiProfile, null, 2));
      setError("");
    } catch (error) {
      console.error("Error fetching Identity provider profile", error);
      setError("Failed to load Identity provider profile");
    } finally {
      setLoading(false);
    }
  }, [profileId]);

  const handleSave = async () => {
    try {
      let profileToSave;
      
      profileToSave = JSON.parse(editorContent);
      const payload = mapUIProfileToApi(profileToSave);
      
      if (isEditMode) {
        await apiClient.put(`/sso-profiles/${profileId}`, payload);
        
        localStorage.setItem(SSO_NOTIFICATION_KEY, JSON.stringify({
          operation: "update",
          message: "Identity provider profile updated successfully",
          timestamp: Date.now()
        }));
        
        navigate("/admin/sso-profiles");
      } else {
        await apiClient.post("/sso-profiles", payload);
        
        localStorage.setItem(SSO_NOTIFICATION_KEY, JSON.stringify({
          operation: "create",
          title: "Your Identity provider profile has been created!",
          message: "After users register with SSO, you'll need to assign them a role in their user details to control their access and permissions.",
          timestamp: Date.now()
        }));

        navigate("/admin/sso-profiles");
      }
    } catch (error) {
      console.error("Error saving Identity provider profile", error);
      setSnackbar({
        open: true,
        message: error.response?.data?.errors?.[0]?.detail || "Failed to save Identity provider profile",
        severity: "error",
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleDeleteClick = () => {
    setWarningDialogOpen(true);
  };

  const handleCancelDelete = () => {
    setWarningDialogOpen(false);
  };

  const handleConfirmDelete = async () => {
    try {
      await apiClient.delete(`/sso-profiles/${profileId}`);
      
      // Store notification data for the listing page
      localStorage.setItem(SSO_NOTIFICATION_KEY, JSON.stringify({
        operation: "delete",
        message: "Identity provider profile deleted successfully",
        timestamp: Date.now()
      }));
      
      navigate("/admin/sso-profiles");
    } catch (error) {
      console.error("Error deleting Identity provider profile", error);
      setSnackbar({
        open: true,
        message: "Failed to delete Identity provider profile",
        severity: "error",
      });
    } finally {
      setWarningDialogOpen(false);
    }
  };
  
  useEffect(() => {
    if (isEditMode && profileId) {
      fetchProfileData();
    }
  }, [isEditMode, fetchProfileData, profileId]);

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
            {isEditMode ? "Edit Profile" : "New Profile"}
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', gap: 2 }}>
          {isEditMode && (
            <DangerOutlineButton
              onClick={handleDeleteClick}
            >
              Delete profile
            </DangerOutlineButton>
          )}
          <PrimaryButton
            variant="contained"
            onClick={handleSave}
          >
            Save profile
          </PrimaryButton>
        </Box>
      </TitleBox>
      <ContentBox>
        <Paper 
          sx={{ 
            height: 'calc(100vh - 117px)', 
            borderRadius: 2, 
            overflow: 'hidden'
          }}
        >
            <Editor
              height="100%"
              defaultLanguage="json"
              value={editorContent}
              onChange={setEditorContent}
              theme="vs-dark"
              options={{
                minimap: { 
                  enabled: true,
                  showSlider: "mouseover",
                  renderCharacters: false,
                  maxColumn: 60,
                  side: "right"
                },
                scrollBeyondLastLine: true,
                formatOnPaste: true,
                formatOnType: true,
                fontSize: 12,
                fontFamily: "'Menlo', 'Monaco', 'Courier New', monospace",
                padding: { top: 12, bottom: 12 },
                lineNumbers: "on",
                lineHeight: 18,
                renderLineHighlight: "all",
                cursorStyle: "line",
                cursorWidth: 2,
                bracketPairColorization: { enabled: true },
                guides: {
                  indentation: true,
                  highlightActiveIndentation: true
                }
              }}
            />
        </Paper>
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
        title="Delete Identity provider profile"
        message="This operation cannot be undone. If you remove this Identity provider profile, all users relying on it won't be able to sign in. Make sure they have another way to log in before proceeding."
        buttonLabel="Delete profile"
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />
    </Box>
  );
};

export default SSOProfileEditor;
