import React, { useState, useEffect, useMemo } from "react";
import apiClient from "../../utils/apiClient";
import { generateSlug } from "../../components/wizards/quick-start/utils";
import {
  TextField,
  Box,
  FormControl,
  FormHelperText,
  InputLabel,
  Select,
  MenuItem,
  Typography,
  Grid,
  Snackbar,
  Alert,
  Switch,
  FormControlLabel,
  Checkbox,
  IconButton,
  Chip,
  Button,
  Card,
  CardContent,
  Divider,
  Tooltip,
  ToggleButton,
  ToggleButtonGroup,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
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
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import PublishSwitch from "../rbac/PublishSwitch";
import { P } from "../../rbac/permissions";
import { useEdition } from "../../context/EditionContext";
import RelationshipPicker from "../common/relationship-picker";
import { listAll } from "../../utils/listAll";
import SemanticRouterTestPanel from "./SemanticRouterTestPanel";
import {
  DEFAULT_THRESHOLD,
  INPUT_SCOPE_LABELS,
  JUDGE_WHEN_LABELS,
  TARGET_LLM,
  TARGET_MODEL_ROUTER,
  apiErrorDetail,
  attributesFromDraft,
  draftFromAttributes,
  emptyDraft,
  emptyRoute,
  routeNames,
  supportsEmbeddings,
  validateDraft,
} from "./semanticRouterModel";

// LLM catalogues come back as JSON:API rows; the picker wants {id, name}.
const catalogueOptions = (rows) =>
  (rows || []).map((c) => ({
    id: Number(c.id),
    name: c.attributes?.name ?? c.name ?? `Catalog ${c.id}`,
  }));

const sameIds = (a, b) => {
  const left = a.map((c) => Number(c.id)).sort((x, y) => x - y);
  const right = b.map((c) => Number(c.id)).sort((x, y) => x - y);
  return left.length === right.length && left.every((id, i) => id === right[i]);
};

// A select over JSON:API rows, by id. `rows` must include the current value
// for it to render with a label.
const RowSelect = ({ id, label, value, onChange, rows, error, helperText, emptyLabel, size }) => (
  <FormControl fullWidth error={Boolean(error)} size={size}>
    <InputLabel id={`${id}-label`}>{label}</InputLabel>
    <Select labelId={`${id}-label`} value={value ? String(value) : ""} label={label} onChange={(e) => onChange(e.target.value)}>
      {emptyLabel && (
        <MenuItem value="">
          <em>{emptyLabel}</em>
        </MenuItem>
      )}
      {rows.map((row) => (
        <MenuItem key={row.id} value={String(row.id)}>
          {row.attributes?.name || `#${row.id}`}
          {row.attributes?.vendor ? ` (${row.attributes.vendor})` : ""}
        </MenuItem>
      ))}
    </Select>
    {(error || helperText) && <FormHelperText>{error || helperText}</FormHelperText>}
  </FormControl>
);

// Keyword chips for one route: type a pattern and press Enter (or Add).
const KeywordEditor = ({ index, keywords, onChange }) => {
  const [pattern, setPattern] = useState("");
  const add = () => {
    const value = pattern.trim();
    if (!value || keywords.some((k) => k.pattern === value)) return;
    onChange([...keywords, { pattern: value, regex: false }]);
    setPattern("");
  };
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "flex-start" }}>
        <TextField
          size="small"
          label="Keyword"
          value={pattern}
          onChange={(e) => setPattern(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          inputProps={{ "aria-label": `Keyword for route ${index + 1}` }}
          helperText="Case-insensitive word or phrase; tick regex for a regular expression."
          sx={{ flexGrow: 1 }}
        />
        <Button size="small" onClick={add} sx={{ mt: 0.5 }}>
          Add keyword
        </Button>
      </Box>
      {keywords.length > 0 && (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mt: 1 }} data-testid={`route-${index}-keywords`}>
          {keywords.map((keyword, ki) => (
            <Box
              key={`${keyword.pattern}-${ki}`}
              sx={{ display: "flex", alignItems: "center", border: 1, borderColor: "divider", borderRadius: 4, pr: 0.5 }}
            >
              <Chip
                label={keyword.regex ? `/${keyword.pattern}/` : keyword.pattern}
                size="small"
                onDelete={() => onChange(keywords.filter((_, i) => i !== ki))}
                sx={{ fontFamily: keyword.regex ? "monospace" : undefined, border: 0 }}
                variant="outlined"
              />
              <FormControlLabel
                sx={{ ml: 0, mr: 0 }}
                control={
                  <Checkbox
                    size="small"
                    checked={keyword.regex}
                    onChange={(e) =>
                      onChange(keywords.map((k, i) => (i === ki ? { ...k, regex: e.target.checked } : k)))
                    }
                  />
                }
                label={<Typography variant="caption">regex</Typography>}
              />
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
};

const RouteCard = ({
  route,
  index,
  count,
  errors,
  llms,
  modelRouters,
  isDefault,
  onUpdate,
  onUpdateTarget,
  onRemove,
  onMove,
}) => (
  <Card variant="outlined" data-testid={`route-card-${index}`}>
    <CardContent>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Typography variant="subtitle1">
            Route {index + 1}
            {route.name ? `: ${route.name}` : ""}
          </Typography>
          {isDefault && <Chip label="Default" size="small" color="primary" />}
        </Box>
        <Box>
          <IconButton size="small" aria-label={`Move route ${index + 1} up`} disabled={index === 0} onClick={() => onMove(index, -1)}>
            <ArrowUpwardIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            aria-label={`Move route ${index + 1} down`}
            disabled={index === count - 1}
            onClick={() => onMove(index, 1)}
          >
            <ArrowDownwardIcon fontSize="small" />
          </IconButton>
          <IconButton size="small" color="error" aria-label={`Remove route ${index + 1}`} onClick={() => onRemove(index)}>
            <DeleteIcon />
          </IconButton>
        </Box>
      </Box>

      <Grid container spacing={2}>
        <Grid item xs={12} md={4}>
          <TextField
            fullWidth
            size="small"
            label="Route name"
            value={route.name}
            onChange={(e) => onUpdate(index, "name", e.target.value)}
            error={Boolean(errors[`route_${index}_name`])}
            helperText={errors[`route_${index}_name`] || "Lowercase, e.g. complex or code_review"}
            required
          />
        </Grid>
        <Grid item xs={12} md={6}>
          <TextField
            fullWidth
            size="small"
            label="Route description"
            value={route.description}
            onChange={(e) => onUpdate(index, "description", e.target.value)}
            helperText="Shown in the portal and given to the LLM judge"
          />
        </Grid>
        <Grid item xs={12} md={2}>
          <TextField
            fullWidth
            size="small"
            label="Priority"
            type="number"
            value={route.priority}
            onChange={(e) => onUpdate(index, "priority", e.target.value)}
            helperText="Breaks keyword ties"
          />
        </Grid>

        {/* Target */}
        <Grid item xs={12}>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Target
          </Typography>
          <ToggleButtonGroup
            size="small"
            exclusive
            value={route.target.type}
            onChange={(_, value) => value && onUpdateTarget(index, { type: value })}
            aria-label={`Target type for route ${index + 1}`}
          >
            <ToggleButton value={TARGET_LLM}>LLM</ToggleButton>
            <ToggleButton value={TARGET_MODEL_ROUTER}>Model Router</ToggleButton>
          </ToggleButtonGroup>
        </Grid>
        {route.target.type === TARGET_MODEL_ROUTER ? (
          <>
            <Grid item xs={12} md={6}>
              <RowSelect
                id={`semanticrouterform-route-${index}-model-router`}
                label="Model router"
                size="small"
                value={route.target.model_router_id}
                onChange={(value) => onUpdateTarget(index, { model_router_id: value })}
                rows={modelRouters}
                error={errors[`route_${index}_target`]}
              />
            </Grid>
            <Grid item xs={12} md={6}>
              <TextField
                fullWidth
                size="small"
                label="Model alias"
                value={route.target.model}
                onChange={(e) => onUpdateTarget(index, { model: e.target.value })}
                error={Boolean(errors[`route_${index}_model`])}
                helperText={errors[`route_${index}_model`] || "The model name the model router receives and matches on its pools"}
                required
              />
            </Grid>
          </>
        ) : (
          <>
            <Grid item xs={12} md={6}>
              <RowSelect
                id={`semanticrouterform-route-${index}-llm`}
                label="LLM provider"
                size="small"
                value={route.target.llm_id}
                onChange={(value) => onUpdateTarget(index, { llm_id: value })}
                rows={llms}
                error={errors[`route_${index}_target`]}
              />
            </Grid>
            <Grid item xs={12} md={6}>
              <TextField
                fullWidth
                size="small"
                label="Model"
                value={route.target.model}
                onChange={(e) => onUpdateTarget(index, { model: e.target.value })}
                error={Boolean(errors[`route_${index}_model`])}
                helperText={errors[`route_${index}_model`] || "e.g. gpt-4o or claude-opus-4"}
                required
              />
            </Grid>
          </>
        )}

        {/* Classification */}
        <Grid item xs={12}>
          <KeywordEditor
            index={index}
            keywords={route.keywords}
            onChange={(keywords) => onUpdate(index, "keywords", keywords)}
          />
        </Grid>
        <Grid item xs={12} md={9}>
          <TextField
            fullWidth
            multiline
            minRows={3}
            size="small"
            label="Example prompts"
            value={route.utterancesText}
            onChange={(e) => onUpdate(index, "utterancesText", e.target.value)}
            helperText="One per line. Prompts similar to these are sent to this route (needs an embedding model)."
            inputProps={{ "aria-label": `Example prompts for route ${index + 1}` }}
          />
        </Grid>
        <Grid item xs={12} md={3}>
          <TextField
            fullWidth
            size="small"
            label="Threshold"
            type="number"
            inputProps={{ step: 0.05, min: 0, max: 1 }}
            value={route.threshold}
            onChange={(e) => onUpdate(index, "threshold", e.target.value)}
            error={Boolean(errors[`route_${index}_threshold`])}
            helperText={errors[`route_${index}_threshold`] || `Similarity needed, 0–1 (default ${DEFAULT_THRESHOLD})`}
          />
        </Grid>
      </Grid>
    </CardContent>
  </Card>
);

const SemanticRouterForm = () => {
  const [router, setRouter] = useState(emptyDraft);
  // LLM catalogues the router is published in. Saved through its own
  // endpoint (PUT /semantic-routers/:id/catalogues) after the router itself.
  const [catalogues, setCatalogues] = useState([]);
  const [selectedCatalogues, setSelectedCatalogues] = useState([]);
  const [savedCatalogues, setSavedCatalogues] = useState([]);
  const [llms, setLLMs] = useState([]);
  const [modelRouters, setModelRouters] = useState([]);
  const [errors, setErrors] = useState({});
  const [saveError, setSaveError] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const navigate = useNavigate();
  const { id } = useParams();
  const isEditMode = !!id;
  const { isEnterprise } = useEdition();

  // Unsaved-changes tracking over the whole router, routes and catalogues
  // included.
  const { markSaved } = useUnsavedForm(
    { router, selectedCatalogues },
    { ready: !isEditMode || loaded },
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/semantic-routers"));

  useEffect(() => {
    fetchLLMs();
    fetchModelRouters();
    fetchCatalogues();
    if (isEditMode) {
      fetchRouter();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const fetchLLMs = async () => {
    try {
      const response = await listAll(apiClient, "/llms");
      setLLMs(response.data.data || []);
    } catch (error) {
      console.error("Error fetching LLMs:", error);
    }
  };

  // Model routers are a sibling Enterprise feature; without them a route can
  // still target an LLM directly.
  const fetchModelRouters = async () => {
    try {
      const response = await listAll(apiClient, "/model-routers");
      setModelRouters(response.data.data || []);
    } catch (error) {
      setModelRouters([]);
    }
  };

  const fetchCatalogues = async () => {
    try {
      const response = await listAll(apiClient, "/catalogues");
      setCatalogues(catalogueOptions(response.data.data));
    } catch (error) {
      console.error("Error fetching catalogues:", error);
    }
  };

  const fetchRouter = async () => {
    try {
      const response = await apiClient.get(`/semantic-routers/${id}`);
      const data = response.data.data;
      setRouter(draftFromAttributes(data.attributes));
      const published = catalogueOptions(data.attributes.catalogues);
      setSelectedCatalogues(published);
      setSavedCatalogues(published);
      setSlugManuallyEdited(true); // Don't auto-generate slug in edit mode
      setLoaded(true);
    } catch (error) {
      console.error("Error fetching semantic router:", error);
      setSnackbar({
        open: true,
        message: "Failed to load semantic router",
        severity: "error",
      });
    }
  };

  const clearError = (key) => {
    if (errors[key]) setErrors((prev) => ({ ...prev, [key]: null }));
  };

  const handleChange = (e) => {
    const { name, value, checked, type } = e.target;
    const newValue = type === "checkbox" ? checked : value;
    setRouter((prev) => ({
      ...prev,
      [name]: newValue,
      // Auto-generate slug from name if not manually edited
      ...(name === "name" && !slugManuallyEdited && { slug: generateSlug(value) }),
    }));
    clearError(name);
  };

  const handleSlugChange = (e) => {
    setSlugManuallyEdited(true);
    handleChange(e);
  };

  // settings.<group>.<field>, or settings.<field> when group is null.
  const updateSetting = (group, field, value) => {
    setRouter((prev) => ({
      ...prev,
      settings: group
        ? { ...prev.settings, [group]: { ...prev.settings[group], [field]: value } }
        : { ...prev.settings, [field]: value },
    }));
    clearError(group || field);
  };

  // Route management
  const addRoute = () => {
    setRouter((prev) => ({ ...prev, routes: [...prev.routes, emptyRoute()] }));
    clearError("routes");
  };

  const removeRoute = (index) => {
    setRouter((prev) => {
      const removed = prev.routes[index]?.name;
      return {
        ...prev,
        routes: prev.routes.filter((_, i) => i !== index),
        // A default that named the removed route no longer points anywhere.
        settings:
          removed && prev.settings.default_route === removed
            ? { ...prev.settings, default_route: "" }
            : prev.settings,
      };
    });
    setErrors({});
  };

  const moveRoute = (index, delta) => {
    setRouter((prev) => {
      const target = index + delta;
      if (target < 0 || target >= prev.routes.length) return prev;
      const routes = [...prev.routes];
      [routes[index], routes[target]] = [routes[target], routes[index]];
      return { ...prev, routes };
    });
    setErrors({});
  };

  const updateRoute = (index, field, value) => {
    setRouter((prev) => {
      const previousName = prev.routes[index]?.name;
      const routes = prev.routes.map((route, i) => (i === index ? { ...route, [field]: value } : route));
      // Renaming the default route keeps it the default.
      const followDefault = field === "name" && previousName && prev.settings.default_route === previousName;
      return {
        ...prev,
        routes,
        settings: followDefault ? { ...prev.settings, default_route: value } : prev.settings,
      };
    });
    clearError(`route_${index}_${field === "name" ? "name" : field}`);
  };

  const updateRouteTarget = (index, patch) => {
    setRouter((prev) => ({
      ...prev,
      routes: prev.routes.map((route, i) =>
        i === index ? { ...route, target: { ...route.target, ...patch } } : route,
      ),
    }));
    clearError(`route_${index}_target`);
    if (patch.model !== undefined) clearError(`route_${index}_model`);
  };

  const names = routeNames(router.routes);

  // The embedding stage only works with vendors that serve embeddings; an
  // already-chosen LLM stays listed so the form shows what is saved.
  const embeddingLLMs = useMemo(
    () =>
      llms.filter(
        (llm) =>
          supportsEmbeddings(llm.attributes?.vendor) ||
          String(llm.id) === String(router.settings.embedding.llm_id),
      ),
    [llms, router.settings.embedding.llm_id],
  );

  const handleSubmit = async () => {
    const newErrors = validateDraft(router, { llms });
    setErrors(newErrors);
    setSaveError("");
    if (Object.keys(newErrors).length > 0) {
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
          type: "semantic-routers",
          attributes: attributesFromDraft(router),
        },
      };

      let routerId = id;
      if (isEditMode) {
        await apiClient.patch(`/semantic-routers/${id}`, payload);
      } else {
        const response = await apiClient.post("/semantic-routers", payload);
        routerId = response?.data?.data?.id;
      }

      // Catalogue membership has its own endpoint, so it is written once the
      // router exists, and only when the selection changed.
      let catalogueFailed = false;
      if (routerId && !sameIds(selectedCatalogues, savedCatalogues)) {
        try {
          await apiClient.put(`/semantic-routers/${routerId}/catalogues`, {
            catalogue_ids: selectedCatalogues.map((c) => Number(c.id)),
          });
        } catch (error) {
          console.error("Error saving semantic router catalogues:", error);
          catalogueFailed = true;
        }
      }

      markSaved();
      navigate("/admin/semantic-routers", {
        state: {
          snackbar: catalogueFailed
            ? {
                message: "Semantic Router saved, but publishing it in the catalogs failed",
                severity: "warning",
              }
            : {
                message: isEditMode
                  ? "Semantic Router updated successfully"
                  : "Semantic Router created successfully",
                severity: "success",
              },
        },
      });
    } catch (error) {
      console.error("Error saving semantic router:", error);
      const detail = apiErrorDetail(error);
      setSaveError(detail);
      setSnackbar({
        open: true,
        message: detail || "Failed to save Semantic Router",
        severity: "error",
      });
    }
  };

  const hasErrors = Object.values(errors).some(Boolean);
  const slugHint = router.slug || "<slug>";

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <SecondaryLinkButton
            component={Link}
            to="/admin/semantic-routers"
            startIcon={<ArrowBackIcon />}
            color="inherit"
          >
            Back
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">
            {isEditMode ? "Edit Semantic Router" : "Create Semantic Router"}
          </Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 2 }}>
          <SecondaryOutlineButton onClick={handleCancel}>
            Cancel
          </SecondaryOutlineButton>
          <PrimaryButton variant="contained" onClick={handleSubmit}>
            {isEditMode ? "Update" : "Create"}
          </PrimaryButton>
        </Box>
      </TitleBox>

      <ContentBox>
        <Grid container spacing={3}>
          {(saveError || hasErrors) && (
            <Grid item xs={12}>
              <Alert severity="error" data-testid="semantic-router-form-errors">
                {saveError || "Some fields need attention; they are marked below."}
              </Alert>
            </Grid>
          )}

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
              helperText={errors.slug || `Clients call it on /v1/chat/completions with the model ${slugHint}/auto. Must not match an LLM's or model router's slug.`}
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
            <PublishSwitch
              permission={P.SEMANTIC_ROUTERS_PUBLISH}
              name="active"
              checked={router.active}
              onChange={handleChange}
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

          {/* Routes */}
          <Grid item xs={12}>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
              <Typography variant="h6">
                Routes
                <Tooltip title="Each prompt goes to one route. Keywords are checked first, then similarity to the example prompts, then (optionally) an LLM judge; otherwise the default route answers.">
                  <InfoOutlinedIcon sx={{ ml: 1, fontSize: 18, color: "text.secondary" }} />
                </Tooltip>
              </Typography>
              <Button variant="outlined" startIcon={<AddIcon />} onClick={addRoute}>
                Add Route
              </Button>
            </Box>
            {errors.routes && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {errors.routes}
              </Alert>
            )}
            {router.routes.length === 0 && !errors.routes && (
              <Alert severity="info" sx={{ mb: 2 }}>
                A semantic router needs at least one route. A route names a target (an LLM and
                model, or a model router) and describes the prompts that belong to it with keywords
                and example prompts. Add a route to get started.
              </Alert>
            )}
          </Grid>

          {router.routes.map((route, index) => (
            <Grid item xs={12} key={index}>
              <RouteCard
                route={route}
                index={index}
                count={router.routes.length}
                errors={errors}
                llms={llms}
                modelRouters={modelRouters}
                isDefault={Boolean(route.name) && route.name === router.settings.default_route}
                onUpdate={updateRoute}
                onUpdateTarget={updateRouteTarget}
                onRemove={removeRoute}
                onMove={moveRoute}
              />
            </Grid>
          ))}

          <Grid item xs={12}>
            <Divider sx={{ my: 2 }} />
          </Grid>

          {/* Classification settings */}
          <Grid item xs={12}>
            <Typography variant="h6" gutterBottom>
              Classification
            </Typography>
          </Grid>

          <Grid item xs={12} md={4}>
            <FormControl fullWidth error={Boolean(errors.default_route)}>
              <InputLabel id="semanticrouterform-default-route-label">Default route</InputLabel>
              <Select
                labelId="semanticrouterform-default-route-label"
                value={names.includes(router.settings.default_route) ? router.settings.default_route : ""}
                label="Default route"
                onChange={(e) => updateSetting(null, "default_route", e.target.value)}
              >
                {names.map((name) => (
                  <MenuItem key={name} value={name}>
                    {name}
                  </MenuItem>
                ))}
              </Select>
              <FormHelperText>
                {errors.default_route || "Answers when no route matches, and every request in shadow mode"}
              </FormHelperText>
            </FormControl>
          </Grid>

          <Grid item xs={12} md={4}>
            <FormControl fullWidth>
              <InputLabel id="semanticrouterform-mode-label">Mode</InputLabel>
              <Select
                labelId="semanticrouterform-mode-label"
                value={router.settings.mode}
                label="Mode"
                onChange={(e) => updateSetting(null, "mode", e.target.value)}
              >
                <MenuItem value="enforce">Enforce: serve the chosen route</MenuItem>
                <MenuItem value="shadow">Shadow: record the choice, serve the default</MenuItem>
              </Select>
            </FormControl>
          </Grid>

          <Grid item xs={12} md={4}>
            <FormControl fullWidth>
              <InputLabel id="semanticrouterform-input-scope-label">Text classified</InputLabel>
              <Select
                labelId="semanticrouterform-input-scope-label"
                value={router.settings.input_scope}
                label="Text classified"
                onChange={(e) => updateSetting(null, "input_scope", e.target.value)}
              >
                {Object.entries(INPUT_SCOPE_LABELS).map(([value, label]) => (
                  <MenuItem key={value} value={value}>
                    {label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>

          <Grid item xs={12} md={6}>
            <FormControlLabel
              control={
                <Switch
                  checked={router.settings.allow_explicit_route}
                  onChange={(e) => updateSetting(null, "allow_explicit_route", e.target.checked)}
                />
              }
              label={`Allow explicit routes (${slugHint}/<route> picks a route directly)`}
            />
          </Grid>

          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label="Max input characters"
              type="number"
              value={router.settings.max_input_chars}
              onChange={(e) => updateSetting(null, "max_input_chars", e.target.value)}
              helperText="Longer text is truncated before classification. Empty uses the default."
            />
          </Grid>

          {/* Embedding */}
          <Grid item xs={12}>
            <Typography variant="subtitle1">Embeddings</Typography>
            <Typography variant="body2" color="text.secondary">
              Needed when any route has example prompts. Only LLM providers whose vendor serves
              embeddings are listed.
            </Typography>
          </Grid>
          <Grid item xs={12} md={6}>
            <RowSelect
              id="semanticrouterform-embedding-llm"
              label="Embedding LLM"
              value={router.settings.embedding.llm_id}
              onChange={(value) => updateSetting("embedding", "llm_id", value)}
              rows={embeddingLLMs}
              emptyLabel="None"
              error={errors.embedding}
            />
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label="Embedding model"
              value={router.settings.embedding.model}
              onChange={(e) => updateSetting("embedding", "model", e.target.value)}
              helperText="e.g. text-embedding-3-small"
              disabled={!router.settings.embedding.llm_id}
            />
          </Grid>

          {/* Judge */}
          <Grid item xs={12}>
            <FormControlLabel
              control={
                <Switch
                  checked={router.settings.judge.enabled}
                  onChange={(e) => updateSetting("judge", "enabled", e.target.checked)}
                />
              }
              label="LLM judge: ask an LLM to pick the route"
            />
            {errors.judge && (
              <Alert severity="error" sx={{ mt: 1 }}>
                {errors.judge}
              </Alert>
            )}
          </Grid>
          {router.settings.judge.enabled && (
            <>
              <Grid item xs={12} md={4}>
                <RowSelect
                  id="semanticrouterform-judge-llm"
                  label="Judge LLM"
                  value={router.settings.judge.llm_id}
                  onChange={(value) => updateSetting("judge", "llm_id", value)}
                  rows={llms}
                />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField
                  fullWidth
                  label="Judge model"
                  value={router.settings.judge.model}
                  onChange={(e) => updateSetting("judge", "model", e.target.value)}
                  helperText="A small, fast model is usually enough"
                />
              </Grid>
              <Grid item xs={12} md={4}>
                <FormControl fullWidth>
                  <InputLabel id="semanticrouterform-judge-when-label">Ask the judge</InputLabel>
                  <Select
                    labelId="semanticrouterform-judge-when-label"
                    value={router.settings.judge.when}
                    label="Ask the judge"
                    onChange={(e) => updateSetting("judge", "when", e.target.value)}
                  >
                    {Object.entries(JUDGE_WHEN_LABELS).map(([value, label]) => (
                      <MenuItem key={value} value={value}>
                        {label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
            </>
          )}

          {/* Affinity */}
          <Grid item xs={12}>
            <FormControlLabel
              control={
                <Switch
                  checked={router.settings.affinity.enabled}
                  onChange={(e) => updateSetting("affinity", "enabled", e.target.checked)}
                />
              }
              label="Session affinity: keep a conversation on the route it started on"
            />
          </Grid>
          {router.settings.affinity.enabled && (
            <>
              <Grid item xs={12} md={6}>
                <TextField
                  fullWidth
                  label="Session header"
                  value={router.settings.affinity.header}
                  onChange={(e) => updateSetting("affinity", "header", e.target.value)}
                  helperText="Request header that identifies the conversation"
                />
              </Grid>
              <Grid item xs={12} md={6}>
                <TextField
                  fullWidth
                  label="Affinity TTL (seconds)"
                  type="number"
                  value={router.settings.affinity.ttl_seconds}
                  onChange={(e) => updateSetting("affinity", "ttl_seconds", e.target.value)}
                  helperText="Empty uses the default (1800)"
                />
              </Grid>
            </>
          )}

          <Grid item xs={12}>
            <Divider sx={{ my: 2 }} />
          </Grid>

          {/* Portal: published in LLM catalogues like an LLM provider, so
              teams holding those catalogues can add it to their Apps. */}
          <Grid item xs={12}>
            <Typography variant="h6" gutterBottom>
              Portal
            </Typography>
            <Typography variant="body2" color="text.secondary">
              How the router appears in the portal catalog. Apps granted the router call the
              unified endpoint with the model written as <code>{slugHint}/auto</code>.
            </Typography>
          </Grid>

          <Grid item xs={12}>
            <RelationshipPicker
              label="Publish in catalogs"
              itemLabel="catalog"
              value={selectedCatalogues}
              onChange={setSelectedCatalogues}
              options={catalogues}
              idField="id"
              getOptionLabel={(c) => c.name}
              helperText="LLM catalogs. Teams holding one of them see the router in the portal and can add it to their Apps."
            />
          </Grid>

          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Short Description"
              name="short_description"
              value={router.short_description}
              onChange={handleChange}
              multiline
              rows={2}
            />
          </Grid>

          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Long Description"
              name="long_description"
              value={router.long_description}
              onChange={handleChange}
              multiline
              rows={4}
            />
          </Grid>

          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Logo URL"
              name="logo_url"
              value={router.logo_url}
              onChange={handleChange}
            />
          </Grid>

          <Grid item xs={12}>
            <Divider sx={{ my: 2 }} />
          </Grid>

          {/* Test the unsaved configuration */}
          <Grid item xs={12}>
            <Typography variant="h6" gutterBottom>
              Test
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Classify a prompt with the configuration above, before saving it.
            </Typography>
            <SemanticRouterTestPanel
              getDraft={() => attributesFromDraft(router)}
              routeNames={names}
              allowExplicit={router.settings.allow_explicit_route}
              llms={llms}
              modelRouters={modelRouters}
            />
          </Grid>
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

export default SemanticRouterForm;
