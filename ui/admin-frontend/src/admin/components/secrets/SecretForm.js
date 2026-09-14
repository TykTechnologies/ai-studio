import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Alert,
  Typography,
  Grid,
  Snackbar,
  IconButton,
  InputAdornment,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import {
  SecondaryLinkButton,
  SecondaryOutlineButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../../styles/sharedStyles";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";

const SecretForm = () => {
  const [formData, setFormData] = useState({
    var_name: "",
    value: "",
  });
  const [loaded, setLoaded] = useState(false);
  const [valueModified, setValueModified] = useState(false);
  const [showSecret, setShowSecret] = useState(false);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  // Unsaved-changes tracking: the baseline is taken once the secret is on
  // screen (or at once for a new secret); markSaved() runs before the
  // post-save redirect so the guard stays quiet on the way out.
  const { markSaved } = useUnsavedForm(formData, { ready: !id || loaded });
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/secrets"));

  useEffect(() => {
    if (id) {
      fetchSecret();
    }
  }, [id]);

  const fetchSecret = async () => {
    try {
      const response = await apiClient.get(`/secrets/${id}`);
      const secretData = response.data.data.attributes;
      setFormData({
        var_name: secretData.var_name,
        value: secretData.value,
      });
      setLoaded(true);
    } catch (error) {
      console.error("Error fetching secret", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch secret details",
        severity: "error",
      });
    }
  };

  const validateForm = () => {
    const newErrors = {};
    if (!formData.var_name.trim()) {
      newErrors.var_name = "Variable name is required";
    }
    if (!id && !formData.value.trim()) {
      newErrors.value = "Secret value is required";
    }

    // Validate variable name format (alphanumeric and underscores only)
    if (formData.var_name && !/^[A-Za-z0-9_]+$/.test(formData.var_name)) {
      newErrors.var_name =
        "Variable name can only contain letters, numbers, and underscores";
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleChange = (field) => (event) => {
    if (field === "value") {
      setValueModified(true);
    }
    setFormData({
      ...formData,
      [field]: event.target.value,
    });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const attributes = {
      var_name: formData.var_name,
    };

    // Only include value if creating a new secret or if the user actually changed it.
    // This prevents overwriting the real encrypted value with the placeholder reference.
    if (!id || valueModified) {
      attributes.value = formData.value;
    }

    const secretData = {
      data: {
        type: "secrets",
        attributes,
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/secrets/${id}`, secretData);
      } else {
        await apiClient.post("/secrets", secretData);
      }

      markSaved();
      navigate("/admin/secrets", {
        state: {
          snackbar: {
            message: id
              ? "Secret updated successfully"
              : "Secret created successfully",
            severity: "success",
          },
        },
      });
    } catch (error) {
      console.error("Error saving secret", error);
      setSnackbar({
        open: true,
        message: "Failed to save secret. Please try again.",
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

  const toggleSecretVisibility = () => {
    setShowSecret(!showSecret);
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {id ? "Edit Secret" : "Add Secret"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/secrets"
          color="inherit"
        >
          Back to secrets
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Variable Name"
                value={formData.var_name}
                onChange={handleChange("var_name")}
                error={!!errors.var_name}
                helperText={
                  errors.var_name ||
                  "Use only letters, numbers, and underscores"
                }
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Secret Value"
                type={showSecret ? "text" : "password"}
                value={formData.value}
                onChange={handleChange("value")}
                error={!!errors.value}
                helperText={errors.value}
                required
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton onClick={toggleSecretVisibility} edge="end">
                        {showSecret ? (
                          <VisibilityOffIcon />
                        ) : (
                          <VisibilityIcon />
                        )}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </Grid>
            <Grid item xs={12}>
              <Box mt={2}>
                <Alert severity="info">
                  This secret will be accessible using:{" "}
                  <code>$SECRET/{formData.var_name}</code>
                </Alert>
              </Box>
            </Grid>
            <Grid item xs={12}>
              <Box display="flex" gap={2}>
                <SecondaryOutlineButton onClick={handleCancel}>
                  Cancel
                </SecondaryOutlineButton>
                <PrimaryButton
                  variant="contained"
                  type="submit"
                  disabled={!formData.var_name || (!id && !formData.value)}
                >
                  {id ? "Update secret" : "Add secret"}
                </PrimaryButton>
              </Box>
            </Grid>
          </Grid>
        </Box>
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

export default SecretForm;
