import React, { useState, useEffect } from "react";
import {
  Typography,
  Box,
  TextField,
  Button,
  CircularProgress,
  Alert,
  Snackbar,
  Paper,
  Divider,
  Collapse,
  IconButton,
} from "@mui/material";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import {
  getBrandingSettings,
  updateBrandingSettings,
  uploadLogo,
  uploadFavicon,
  resetBrandingToDefaults,
  getLogoUrl,
  getFaviconUrl,
} from "../services/brandingService";
import SaveIcon from "@mui/icons-material/Save";
import RefreshIcon from "@mui/icons-material/Refresh";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import CloudUploadIcon from "@mui/icons-material/CloudUpload";

const BrandingSettings = () => {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [settings, setSettings] = useState({
    app_title: "",
    primary_color: "#23E2C2",
    secondary_color: "#343452",
    background_color: "#FFFFFF",
    custom_css: "",
  });
  const [logoFile, setLogoFile] = useState(null);
  const [faviconFile, setFaviconFile] = useState(null);
  const [logoPreview, setLogoPreview] = useState(null);
  const [faviconPreview, setFaviconPreview] = useState(null);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [errors, setErrors] = useState({});

  useEffect(() => {
    fetchBrandingSettings();
  }, []);

  const fetchBrandingSettings = async () => {
    try {
      setLoading(true);
      const response = await getBrandingSettings();
      // API returns JSON:API format with attributes nested
      const data = response.attributes || response;

      setSettings({
        app_title: data.app_title || "",
        primary_color: data.primary_color || "#23E2C2",
        secondary_color: data.secondary_color || "#343452",
        background_color: data.background_color || "#FFFFFF",
        custom_css: data.custom_css || "",
      });

      // Set previews if custom assets exist
      if (data.has_custom_logo) {
        setLogoPreview(`${getLogoUrl()}?t=${Date.now()}`);
      }
      if (data.has_custom_favicon) {
        setFaviconPreview(`${getFaviconUrl()}?t=${Date.now()}`);
      }
    } catch (error) {
      setSnackbar({
        open: true,
        message: "Failed to load branding settings",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const validateSettings = () => {
    const newErrors = {};

    // Validate app title length
    if (settings.app_title && settings.app_title.length > 50) {
      newErrors.app_title = "App title must be 50 characters or less";
    }

    // Validate hex colors
    const hexColorRegex = /^#[0-9A-Fa-f]{6}$/;
    if (settings.primary_color && !hexColorRegex.test(settings.primary_color)) {
      newErrors.primary_color = "Must be a valid hex color (e.g., #23E2C2)";
    }
    if (settings.secondary_color && !hexColorRegex.test(settings.secondary_color)) {
      newErrors.secondary_color = "Must be a valid hex color (e.g., #343452)";
    }
    if (settings.background_color && !hexColorRegex.test(settings.background_color)) {
      newErrors.background_color = "Must be a valid hex color (e.g., #FFFFFF)";
    }

    // Validate logo file size (2MB max)
    if (logoFile && logoFile.size > 2 * 1024 * 1024) {
      newErrors.logo = "Logo file must be 2MB or less";
    }

    // Validate favicon file size (100KB max)
    if (faviconFile && faviconFile.size > 100 * 1024) {
      newErrors.favicon = "Favicon file must be 100KB or less";
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleInputChange = (field) => (event) => {
    setSettings({
      ...settings,
      [field]: event.target.value,
    });
    // Clear error for this field
    if (errors[field]) {
      setErrors({
        ...errors,
        [field]: undefined,
      });
    }
  };

  const handleLogoChange = (event) => {
    const file = event.target.files[0];
    if (file) {
      setLogoFile(file);
      const reader = new FileReader();
      reader.onloadend = () => {
        setLogoPreview(reader.result);
      };
      reader.readAsDataURL(file);
      // Clear error
      if (errors.logo) {
        setErrors({ ...errors, logo: undefined });
      }
    }
  };

  const handleFaviconChange = (event) => {
    const file = event.target.files[0];
    if (file) {
      setFaviconFile(file);
      const reader = new FileReader();
      reader.onloadend = () => {
        setFaviconPreview(reader.result);
      };
      reader.readAsDataURL(file);
      // Clear error
      if (errors.favicon) {
        setErrors({ ...errors, favicon: undefined });
      }
    }
  };

  const handleSave = async () => {
    if (!validateSettings()) {
      setSnackbar({
        open: true,
        message: "Please fix validation errors before saving",
        severity: "error",
      });
      return;
    }

    try {
      setSaving(true);

      // Upload logo if changed
      if (logoFile) {
        await uploadLogo(logoFile);
      }

      // Upload favicon if changed
      if (faviconFile) {
        await uploadFavicon(faviconFile);
      }

      // Update settings
      await updateBrandingSettings(settings);

      setSnackbar({
        open: true,
        message: "Branding settings saved successfully. Refresh the page to see changes.",
        severity: "success",
      });

      // Clear file selections
      setLogoFile(null);
      setFaviconFile(null);
    } catch (error) {
      setSnackbar({
        open: true,
        message: error.message || "Failed to save branding settings",
        severity: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    if (!window.confirm("Are you sure you want to reset all branding to defaults? This action cannot be undone.")) {
      return;
    }

    try {
      setSaving(true);
      await resetBrandingToDefaults();

      setSnackbar({
        open: true,
        message: "Branding reset to defaults successfully. Refresh the page to see changes.",
        severity: "success",
      });

      // Reload settings
      await fetchBrandingSettings();

      // Clear file selections
      setLogoFile(null);
      setFaviconFile(null);
      setLogoPreview(null);
      setFaviconPreview(null);
    } catch (error) {
      setSnackbar({
        open: true,
        message: error.message || "Failed to reset branding settings",
        severity: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Branding Settings</Typography>
        <Box display="flex" gap={2}>
          <Button
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={handleReset}
            disabled={saving}
          >
            Reset to Defaults
          </Button>
          <PrimaryButton
            variant="contained"
            startIcon={<SaveIcon />}
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? "Saving..." : "Save Changes"}
          </PrimaryButton>
        </Box>
      </TitleBox>

      <ContentBox>
        <StyledPaper>
          <Box p={3}>
            {/* Logo Upload Section */}
            <Typography variant="h6" gutterBottom>
              Logo
            </Typography>
            <Typography variant="body2" color="text.secondary" paragraph>
              Upload a custom logo for the header navigation. Supported formats: PNG, JPG, SVG. Max size: 2MB.
            </Typography>
            <Box display="flex" gap={3} alignItems="center" mb={4}>
              <Box>
                <input
                  accept="image/png,image/jpeg,image/svg+xml"
                  style={{ display: "none" }}
                  id="logo-upload"
                  type="file"
                  onChange={handleLogoChange}
                />
                <label htmlFor="logo-upload">
                  <Button
                    variant="outlined"
                    component="span"
                    startIcon={<CloudUploadIcon />}
                  >
                    Upload Logo
                  </Button>
                </label>
                {errors.logo && (
                  <Typography variant="caption" color="error" display="block" mt={1}>
                    {errors.logo}
                  </Typography>
                )}
              </Box>
              {logoPreview && (
                <Box>
                  <Typography variant="caption" color="text.secondary" display="block" mb={1}>
                    Preview:
                  </Typography>
                  <Paper
                    elevation={1}
                    sx={{
                      p: 2,
                      display: "inline-block",
                      maxWidth: 200,
                      backgroundColor: "background.default",
                    }}
                  >
                    <img
                      src={logoPreview}
                      alt="Logo preview"
                      style={{ maxWidth: "100%", maxHeight: 60, display: "block" }}
                    />
                  </Paper>
                </Box>
              )}
            </Box>

            <Divider sx={{ my: 3 }} />

            {/* Favicon Upload Section */}
            <Typography variant="h6" gutterBottom>
              Favicon
            </Typography>
            <Typography variant="body2" color="text.secondary" paragraph>
              Upload a custom favicon (browser tab icon). Supported formats: ICO, PNG. Max size: 100KB.
            </Typography>
            <Box display="flex" gap={3} alignItems="center" mb={4}>
              <Box>
                <input
                  accept="image/x-icon,image/png"
                  style={{ display: "none" }}
                  id="favicon-upload"
                  type="file"
                  onChange={handleFaviconChange}
                />
                <label htmlFor="favicon-upload">
                  <Button
                    variant="outlined"
                    component="span"
                    startIcon={<CloudUploadIcon />}
                  >
                    Upload Favicon
                  </Button>
                </label>
                {errors.favicon && (
                  <Typography variant="caption" color="error" display="block" mt={1}>
                    {errors.favicon}
                  </Typography>
                )}
              </Box>
              {faviconPreview && (
                <Box>
                  <Typography variant="caption" color="text.secondary" display="block" mb={1}>
                    Preview:
                  </Typography>
                  <Paper
                    elevation={1}
                    sx={{
                      p: 2,
                      display: "inline-block",
                      backgroundColor: "background.default",
                    }}
                  >
                    <img
                      src={faviconPreview}
                      alt="Favicon preview"
                      style={{ width: 32, height: 32, display: "block" }}
                    />
                  </Paper>
                </Box>
              )}
            </Box>

            <Divider sx={{ my: 3 }} />

            {/* Color Customization */}
            <Typography variant="h6" gutterBottom>
              Color Scheme
            </Typography>
            <Typography variant="body2" color="text.secondary" paragraph>
              Customize the color palette of the application. Use hex color codes (e.g., #23E2C2).
            </Typography>
            <Box display="flex" flexDirection="column" gap={2} mb={4}>
              <Box display="flex" gap={2} alignItems="center">
                <TextField
                  label="Primary Color"
                  value={settings.primary_color}
                  onChange={handleInputChange("primary_color")}
                  error={!!errors.primary_color}
                  helperText={errors.primary_color}
                  placeholder="#23E2C2"
                  sx={{ width: 200 }}
                />
                <Box
                  sx={{
                    width: 50,
                    height: 50,
                    backgroundColor: settings.primary_color,
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 1,
                  }}
                />
              </Box>
              <Box display="flex" gap={2} alignItems="center">
                <TextField
                  label="Secondary Color"
                  value={settings.secondary_color}
                  onChange={handleInputChange("secondary_color")}
                  error={!!errors.secondary_color}
                  helperText={errors.secondary_color}
                  placeholder="#343452"
                  sx={{ width: 200 }}
                />
                <Box
                  sx={{
                    width: 50,
                    height: 50,
                    backgroundColor: settings.secondary_color,
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 1,
                  }}
                />
              </Box>
              <Box display="flex" gap={2} alignItems="center">
                <TextField
                  label="Background Color"
                  value={settings.background_color}
                  onChange={handleInputChange("background_color")}
                  error={!!errors.background_color}
                  helperText={errors.background_color}
                  placeholder="#FFFFFF"
                  sx={{ width: 200 }}
                />
                <Box
                  sx={{
                    width: 50,
                    height: 50,
                    backgroundColor: settings.background_color,
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 1,
                  }}
                />
              </Box>
            </Box>

            <Divider sx={{ my: 3 }} />

            {/* App Title */}
            <Typography variant="h6" gutterBottom>
              Application Title
            </Typography>
            <Typography variant="body2" color="text.secondary" paragraph>
              Customize the application title shown in the browser tab.
            </Typography>
            <TextField
              label="App Title"
              value={settings.app_title}
              onChange={handleInputChange("app_title")}
              error={!!errors.app_title}
              helperText={errors.app_title || `${settings.app_title.length}/50 characters`}
              placeholder="Tyk AI Portal"
              fullWidth
              sx={{ mb: 4 }}
            />

            <Divider sx={{ my: 3 }} />

            {/* Custom CSS (Advanced) */}
            <Box>
              <Box display="flex" alignItems="center" justifyContent="space-between" mb={2}>
                <Box>
                  <Typography variant="h6">
                    Custom CSS (Advanced)
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Add custom CSS to override default styles. Use with caution.
                  </Typography>
                </Box>
                <IconButton onClick={() => setShowAdvanced(!showAdvanced)}>
                  {showAdvanced ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                </IconButton>
              </Box>
              <Collapse in={showAdvanced}>
                <Alert severity="warning" sx={{ mb: 2 }}>
                  Custom CSS can affect application appearance and functionality. Test thoroughly before saving.
                </Alert>
                <TextField
                  label="Custom CSS"
                  value={settings.custom_css}
                  onChange={handleInputChange("custom_css")}
                  multiline
                  rows={10}
                  fullWidth
                  placeholder=".custom-class { color: red; }"
                  sx={{ fontFamily: "monospace" }}
                />
              </Collapse>
            </Box>
          </Box>
        </StyledPaper>
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
    </>
  );
};

export default BrandingSettings;
