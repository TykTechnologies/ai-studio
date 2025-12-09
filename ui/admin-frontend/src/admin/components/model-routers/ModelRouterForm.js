import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import { generateSlug } from "../../components/wizards/quick-start/utils";
import {
  TextField,
  Box,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Typography,
  Grid,
  Snackbar,
  Alert,
  Switch,
  FormControlLabel,
  IconButton,
  AccordionSummary,
  AccordionDetails,
  Chip,
  Button,
  Card,
  CardContent,
  CardActions,
  Divider,
  Tooltip,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import { useEdition } from "../../context/EditionContext";

const ModelRouterForm = () => {
  const [router, setRouter] = useState({
    name: "",
    slug: "",
    description: "",
    api_compat: "openai",
    active: false,
    namespace: "",
    pools: [],
  });
  const [availableLLMs, setAvailableLLMs] = useState([]);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false);

  const navigate = useNavigate();
  const { id } = useParams();
  const isEditMode = !!id;
  const { isEnterprise } = useEdition();

  useEffect(() => {
    fetchLLMs();
    if (isEditMode) {
      fetchRouter();
    }
  }, [id]);

  const fetchLLMs = async () => {
    try {
      const response = await apiClient.get("/llms", { params: { all: true } });
      setAvailableLLMs(response.data.data || []);
    } catch (error) {
      console.error("Error fetching LLMs:", error);
    }
  };

  const fetchRouter = async () => {
    try {
      const response = await apiClient.get(`/model-routers/${id}`);
      const data = response.data.data;
      setRouter({
        name: data.attributes.name || "",
        slug: data.attributes.slug || "",
        description: data.attributes.description || "",
        api_compat: data.attributes.api_compat || "openai",
        active: data.attributes.active || false,
        namespace: data.attributes.namespace || "",
        pools: data.attributes.pools || [],
      });
      setSlugManuallyEdited(true); // Don't auto-generate slug in edit mode
    } catch (error) {
      console.error("Error fetching router:", error);
      setSnackbar({
        open: true,
        message: "Failed to load router",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value, checked, type } = e.target;
    const newValue = type === "checkbox" ? checked : value;

    setRouter((prev) => ({
      ...prev,
      [name]: newValue,
    }));

    // Auto-generate slug from name if not manually edited
    if (name === "name" && !slugManuallyEdited) {
      setRouter((prev) => ({
        ...prev,
        slug: generateSlug(value),
      }));
    }

    // Clear error when field is modified
    if (errors[name]) {
      setErrors((prev) => ({ ...prev, [name]: null }));
    }
  };

  const handleSlugChange = (e) => {
    setSlugManuallyEdited(true);
    handleChange(e);
  };

  // Pool management
  const addPool = () => {
    setRouter((prev) => ({
      ...prev,
      pools: [
        ...prev.pools,
        {
          name: "",
          model_pattern: "*",
          selection_algorithm: "round_robin",
          priority: prev.pools.length,
          vendors: [],
        },
      ],
    }));
  };

  const removePool = (index) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.filter((_, i) => i !== index),
    }));
  };

  const updatePool = (index, field, value) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === index ? { ...pool, [field]: value } : pool
      ),
    }));
  };

  // Vendor management within pools
  const addVendor = (poolIndex) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === poolIndex
          ? {
              ...pool,
              vendors: [
                ...pool.vendors,
                { llm_id: "", llm_slug: "", weight: 1, is_active: true, mappings: [] },
              ],
            }
          : pool
      ),
    }));
  };

  const removeVendor = (poolIndex, vendorIndex) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === poolIndex
          ? {
              ...pool,
              vendors: pool.vendors.filter((_, vi) => vi !== vendorIndex),
            }
          : pool
      ),
    }));
  };

  const updateVendor = (poolIndex, vendorIndex, field, value) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === poolIndex
          ? {
              ...pool,
              vendors: pool.vendors.map((vendor, vi) => {
                if (vi !== vendorIndex) return vendor;
                // If updating llm_id, also update llm_slug
                if (field === "llm_id") {
                  const selectedLLM = availableLLMs.find(
                    (llm) => llm.id === parseInt(value)
                  );
                  return {
                    ...vendor,
                    llm_id: parseInt(value),
                    llm_slug: selectedLLM?.attributes?.slug || "",
                  };
                }
                return { ...vendor, [field]: value };
              }),
            }
          : pool
      ),
    }));
  };

  // Vendor-specific model mapping management
  const addVendorMapping = (poolIndex, vendorIndex) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === poolIndex
          ? {
              ...pool,
              vendors: pool.vendors.map((vendor, vi) =>
                vi === vendorIndex
                  ? {
                      ...vendor,
                      mappings: [...(vendor.mappings || []), { source_model: "", target_model: "" }],
                    }
                  : vendor
              ),
            }
          : pool
      ),
    }));
  };

  const removeVendorMapping = (poolIndex, vendorIndex, mappingIndex) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === poolIndex
          ? {
              ...pool,
              vendors: pool.vendors.map((vendor, vi) =>
                vi === vendorIndex
                  ? {
                      ...vendor,
                      mappings: (vendor.mappings || []).filter((_, mi) => mi !== mappingIndex),
                    }
                  : vendor
              ),
            }
          : pool
      ),
    }));
  };

  const updateVendorMapping = (poolIndex, vendorIndex, mappingIndex, field, value) => {
    setRouter((prev) => ({
      ...prev,
      pools: prev.pools.map((pool, i) =>
        i === poolIndex
          ? {
              ...pool,
              vendors: pool.vendors.map((vendor, vi) =>
                vi === vendorIndex
                  ? {
                      ...vendor,
                      mappings: (vendor.mappings || []).map((mapping, mi) =>
                        mi === mappingIndex ? { ...mapping, [field]: value } : mapping
                      ),
                    }
                  : vendor
              ),
            }
          : pool
      ),
    }));
  };

  const validateForm = () => {
    const newErrors = {};
    if (!router.name.trim()) {
      newErrors.name = "Name is required";
    }
    if (!router.slug.trim()) {
      newErrors.slug = "Slug is required";
    }
    if (router.pools.length === 0) {
      newErrors.pools = "At least one pool is required";
    }
    router.pools.forEach((pool, index) => {
      if (!pool.name.trim()) {
        newErrors[`pool_${index}_name`] = "Pool name is required";
      }
      if (!pool.model_pattern.trim()) {
        newErrors[`pool_${index}_pattern`] = "Model pattern is required";
      }
      if (pool.vendors.length === 0) {
        newErrors[`pool_${index}_vendors`] = "At least one vendor is required";
      }
    });
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async () => {
    if (!validateForm()) {
      setSnackbar({
        open: true,
        message: "Please fix the validation errors",
        severity: "error",
      });
      return;
    }

    try {
      const payload = {
        data: {
          type: "model_router",
          attributes: {
            name: router.name,
            slug: router.slug,
            description: router.description,
            api_compat: router.api_compat,
            active: router.active,
            namespace: router.namespace,
            pools: router.pools.map((pool, index) => ({
              name: pool.name,
              model_pattern: pool.model_pattern,
              selection_algorithm: pool.selection_algorithm,
              priority: pool.priority || index,
              vendors: pool.vendors.map((v) => ({
                llm_id: v.llm_id,
                llm_slug: v.llm_slug,
                weight: parseInt(v.weight) || 1,
                is_active: v.is_active !== false,
                mappings: (v.mappings || []).filter(
                  (m) => m.source_model && m.target_model
                ),
              })),
            })),
          },
        },
      };

      if (isEditMode) {
        await apiClient.patch(`/model-routers/${id}`, payload);
        setSnackbar({
          open: true,
          message: "Model Router updated successfully",
          severity: "success",
        });
      } else {
        await apiClient.post("/model-routers", payload);
        setSnackbar({
          open: true,
          message: "Model Router created successfully",
          severity: "success",
        });
      }

      setTimeout(() => navigate("/admin/model-routers"), 1500);
    } catch (error) {
      console.error("Error saving router:", error);
      setSnackbar({
        open: true,
        message: error.response?.data?.message || "Failed to save Model Router",
        severity: "error",
      });
    }
  };

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <SecondaryLinkButton
            component={Link}
            to="/admin/model-routers"
            startIcon={<ArrowBackIcon />}
            color="inherit"
          >
            Back
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">
            {isEditMode ? "Edit Model Router" : "Create Model Router"}
          </Typography>
        </Box>
        <PrimaryButton variant="contained" onClick={handleSubmit}>
          {isEditMode ? "Update" : "Create"}
        </PrimaryButton>
      </TitleBox>

      <ContentBox>
        <Grid container spacing={3}>
          {/* Basic Information */}
          <Grid item xs={12}>
            <Typography variant="h6" gutterBottom>
              Basic Information
            </Typography>
          </Grid>

          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label="Name"
              name="name"
              value={router.name}
              onChange={handleChange}
              error={!!errors.name}
              helperText={errors.name}
              required
            />
          </Grid>

          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label="Slug"
              name="slug"
              value={router.slug}
              onChange={handleSlugChange}
              error={!!errors.slug}
              helperText={errors.slug || "Used in URL: /router/{slug}/v1/chat/completions"}
              required
            />
          </Grid>

          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Description"
              name="description"
              value={router.description}
              onChange={handleChange}
              multiline
              rows={2}
            />
          </Grid>

          <Grid item xs={12} md={6}>
            <FormControl fullWidth>
              <InputLabel>API Compatibility</InputLabel>
              <Select
                name="api_compat"
                value={router.api_compat}
                onChange={handleChange}
                label="API Compatibility"
              >
                <MenuItem value="openai">OpenAI</MenuItem>
              </Select>
            </FormControl>
          </Grid>

          <Grid item xs={12} md={6}>
            <FormControlLabel
              control={
                <Switch
                  name="active"
                  checked={router.active}
                  onChange={handleChange}
                />
              }
              label="Active"
            />
          </Grid>

          {/* Edge Availability */}
          {isEnterprise && (
            <Grid item xs={12}>
              <EdgeAvailabilitySection
                namespace={router.namespace}
                onChange={(namespace) =>
                  setRouter((prev) => ({ ...prev, namespace }))
                }
              />
            </Grid>
          )}

          <Grid item xs={12}>
            <Divider sx={{ my: 2 }} />
          </Grid>

          {/* Pools Section */}
          <Grid item xs={12}>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
              <Typography variant="h6">
                Model Pools
                <Tooltip title="Pools match incoming requests by model name pattern and route to configured vendors">
                  <InfoOutlinedIcon sx={{ ml: 1, fontSize: 18, color: "text.secondary" }} />
                </Tooltip>
              </Typography>
              <Button
                variant="outlined"
                startIcon={<AddIcon />}
                onClick={addPool}
              >
                Add Pool
              </Button>
            </Box>
            {errors.pools && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {errors.pools}
              </Alert>
            )}
          </Grid>

          {router.pools.map((pool, poolIndex) => (
            <Grid item xs={12} key={poolIndex}>
              <Card variant="outlined">
                <CardContent>
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
                    <Typography variant="subtitle1">
                      Pool {poolIndex + 1}
                    </Typography>
                    <IconButton
                      color="error"
                      onClick={() => removePool(poolIndex)}
                      size="small"
                    >
                      <DeleteIcon />
                    </IconButton>
                  </Box>

                  <Grid container spacing={2}>
                    <Grid item xs={12} md={4}>
                      <TextField
                        fullWidth
                        label="Pool Name"
                        value={pool.name}
                        onChange={(e) => updatePool(poolIndex, "name", e.target.value)}
                        error={!!errors[`pool_${poolIndex}_name`]}
                        helperText={errors[`pool_${poolIndex}_name`]}
                        size="small"
                        required
                      />
                    </Grid>

                    <Grid item xs={12} md={4}>
                      <TextField
                        fullWidth
                        label="Model Pattern"
                        value={pool.model_pattern}
                        onChange={(e) => updatePool(poolIndex, "model_pattern", e.target.value)}
                        error={!!errors[`pool_${poolIndex}_pattern`]}
                        helperText={errors[`pool_${poolIndex}_pattern`] || "Glob pattern: claude-*, gpt-4*, *"}
                        size="small"
                        required
                      />
                    </Grid>

                    <Grid item xs={12} md={2}>
                      <FormControl fullWidth size="small">
                        <InputLabel>Algorithm</InputLabel>
                        <Select
                          value={pool.selection_algorithm}
                          onChange={(e) => updatePool(poolIndex, "selection_algorithm", e.target.value)}
                          label="Algorithm"
                        >
                          <MenuItem value="round_robin">Round Robin</MenuItem>
                          <MenuItem value="weighted">Weighted</MenuItem>
                        </Select>
                      </FormControl>
                    </Grid>

                    <Grid item xs={12} md={2}>
                      <TextField
                        fullWidth
                        label="Priority"
                        type="number"
                        value={pool.priority}
                        onChange={(e) => updatePool(poolIndex, "priority", parseInt(e.target.value) || 0)}
                        helperText="Higher = checked first"
                        size="small"
                      />
                    </Grid>

                    {/* Vendors */}
                    <Grid item xs={12}>
                      <Box sx={{ mt: 2 }}>
                        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                          <Typography variant="body2" color="text.secondary">
                            Vendors
                          </Typography>
                          <Button
                            size="small"
                            startIcon={<AddIcon />}
                            onClick={() => addVendor(poolIndex)}
                          >
                            Add Vendor
                          </Button>
                        </Box>
                        {errors[`pool_${poolIndex}_vendors`] && (
                          <Alert severity="error" sx={{ mb: 1 }}>
                            {errors[`pool_${poolIndex}_vendors`]}
                          </Alert>
                        )}
                        {pool.vendors.map((vendor, vendorIndex) => (
                          <Card
                            key={vendorIndex}
                            variant="outlined"
                            sx={{ mb: 2, bgcolor: "grey.50" }}
                          >
                            <CardContent sx={{ py: 1.5, "&:last-child": { pb: 1.5 } }}>
                              <Box
                                sx={{
                                  display: "flex",
                                  gap: 2,
                                  alignItems: "center",
                                  mb: 1,
                                }}
                              >
                                <FormControl sx={{ minWidth: 200 }} size="small">
                                  <InputLabel>LLM</InputLabel>
                                  <Select
                                    value={vendor.llm_id || ""}
                                    onChange={(e) => updateVendor(poolIndex, vendorIndex, "llm_id", e.target.value)}
                                    label="LLM"
                                  >
                                    {availableLLMs.map((llm) => (
                                      <MenuItem key={llm.id} value={llm.id}>
                                        {llm.attributes.name}
                                      </MenuItem>
                                    ))}
                                  </Select>
                                </FormControl>
                                {pool.selection_algorithm === "weighted" && (
                                  <TextField
                                    label="Weight"
                                    type="number"
                                    value={vendor.weight}
                                    onChange={(e) => updateVendor(poolIndex, vendorIndex, "weight", e.target.value)}
                                    size="small"
                                    sx={{ width: 100 }}
                                  />
                                )}
                                <FormControlLabel
                                  control={
                                    <Switch
                                      checked={vendor.is_active !== false}
                                      onChange={(e) => updateVendor(poolIndex, vendorIndex, "is_active", e.target.checked)}
                                      size="small"
                                    />
                                  }
                                  label="Active"
                                />
                                <IconButton
                                  color="error"
                                  onClick={() => removeVendor(poolIndex, vendorIndex)}
                                  size="small"
                                >
                                  <DeleteIcon />
                                </IconButton>
                              </Box>

                              {/* Vendor-specific Model Mappings */}
                              <StyledAccordion sx={{ mt: 1 }}>
                                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                                  <Typography variant="caption">
                                    Model Mappings ({(vendor.mappings || []).length})
                                  </Typography>
                                </AccordionSummary>
                                <AccordionDetails>
                                  <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: "block" }}>
                                    Rename models when routing to this vendor (e.g., map "gpt-4" to "claude-3-opus")
                                  </Typography>
                                  <Button
                                    size="small"
                                    startIcon={<AddIcon />}
                                    onClick={() => addVendorMapping(poolIndex, vendorIndex)}
                                    sx={{ mb: 1 }}
                                  >
                                    Add Mapping
                                  </Button>
                                  {(vendor.mappings || []).map((mapping, mappingIndex) => (
                                    <Box
                                      key={mappingIndex}
                                      sx={{
                                        display: "flex",
                                        gap: 1,
                                        alignItems: "center",
                                        mb: 1,
                                      }}
                                    >
                                      <TextField
                                        label="Source"
                                        value={mapping.source_model}
                                        onChange={(e) => updateVendorMapping(poolIndex, vendorIndex, mappingIndex, "source_model", e.target.value)}
                                        size="small"
                                        placeholder="gpt-4"
                                        sx={{ width: 150 }}
                                      />
                                      <Typography variant="body2">→</Typography>
                                      <TextField
                                        label="Target"
                                        value={mapping.target_model}
                                        onChange={(e) => updateVendorMapping(poolIndex, vendorIndex, mappingIndex, "target_model", e.target.value)}
                                        size="small"
                                        placeholder="claude-3-opus"
                                        sx={{ width: 150 }}
                                      />
                                      <IconButton
                                        color="error"
                                        onClick={() => removeVendorMapping(poolIndex, vendorIndex, mappingIndex)}
                                        size="small"
                                      >
                                        <DeleteIcon />
                                      </IconButton>
                                    </Box>
                                  ))}
                                </AccordionDetails>
                              </StyledAccordion>
                            </CardContent>
                          </Card>
                        ))}
                      </Box>
                    </Grid>

                  </Grid>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
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
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default ModelRouterForm;
