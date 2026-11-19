import React, { useState, useEffect } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Typography,
  Snackbar,
  Alert,
  CircularProgress,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import RelationshipPicker from "../common/relationship-picker";
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
import { listAll } from "../../utils/listAll";
import useSystemFeatures from "../../hooks/useSystemFeatures";
import { P } from "../../rbac/permissions";
import WorkspaceSelect from "../workspaces/WorkspaceSelect";
import { useCanMove, workspaceFieldError } from "../workspaces/workspaceFields";
import NotFound from "../../pages/NotFound";
import OtherWorkspacesNote from "./OtherWorkspacesNote";

// Routers are published in LLM catalogues too; the picker works on
// {id, attributes: {name}} items like the LLM one.
const routerOption = (r) => ({ id: String(r.id), attributes: { name: r.name } });

const CatalogueForm = () => {
  const [catalogue, setCatalogue] = useState({ name: "" });
  // Enterprise: the Workspace it lives in (landing rule when null). The
  // stored one is kept apart: a different workspace_id on save is a move.
  const [workspaceId, setWorkspaceId] = useState(null);
  const [originalWorkspaceId, setOriginalWorkspaceId] = useState(null);
  const [workspaceError, setWorkspaceError] = useState("");
  const canMove = useCanMove(P.CATALOGUES_WRITE);
  // The catalog is gone or in a Workspace the caller cannot see.
  const [notFound, setNotFound] = useState(false);
  const [llms, setLLMs] = useState([]);
  const [availableLLMs, setAvailableLLMs] = useState([]);
  const [modelRouters, setModelRouters] = useState([]);
  const [semanticRouters, setSemanticRouters] = useState([]);
  const [availableModelRouters, setAvailableModelRouters] = useState([]);
  const [availableSemanticRouters, setAvailableSemanticRouters] = useState([]);
  const { features } = useSystemFeatures();
  const showModelRouters = Boolean(features?.feature_model_router);
  const showSemanticRouters = Boolean(features?.feature_semantic_router);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const { id } = useParams();

  // Unsaved-changes tracking: the name plus the picker's membership.
  const { markSaved } = useUnsavedForm(
    {
      name: catalogue.name,
      llmIds: llms.map((llm) => String(llm.id)).sort(),
      modelRouterIds: modelRouters.map((r) => String(r.id)).sort(),
      semanticRouterIds: semanticRouters.map((r) => String(r.id)).sort(),
    },
    { ready: !loading }
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/catalogs/llms"));

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [catalogueResponse, llmsResponse] = await Promise.all([
          id ? apiClient.get(`/catalogues/${id}`) : Promise.resolve(null),
          listAll(apiClient, "/llms"),
        ]);

        if (catalogueResponse) {
          setCatalogue(catalogueResponse.data.data.attributes);
          setWorkspaceId(catalogueResponse.data.data.attributes.workspace_id ?? null);
          setOriginalWorkspaceId(catalogueResponse.data.data.attributes.workspace_id ?? null);
          const catalogueLLMsResponse = await apiClient.get(
            `/catalogues/${id}/llms`,
          );
          setLLMs(catalogueLLMsResponse.data.data);
        }

        setAvailableLLMs(
          llmsResponse.data.data.filter((llm) => llm.attributes.active),
        );
      } catch (error) {
        console.error("Error fetching data", error);
        if (error.response?.status === 404) {
          setNotFound(true);
        }
        setSnackbar({
          open: true,
          message: "Error fetching data",
          severity: "error",
        });
      }
      setLoading(false);
    };

    fetchData();
  }, [id]);

  // Routers load once the edition says they exist.
  useEffect(() => {
    if (!showModelRouters && !showSemanticRouters) return;
    let cancelled = false;
    const fetchRouters = async () => {
      try {
        const [current, mrs, srs] = await Promise.all([
          id ? apiClient.get(`/catalogues/${id}/routers`) : Promise.resolve(null),
          showModelRouters ? listAll(apiClient, "/model-routers") : Promise.resolve(null),
          showSemanticRouters ? listAll(apiClient, "/semantic-routers") : Promise.resolve(null),
        ]);
        if (cancelled) return;
        if (mrs) setAvailableModelRouters(mrs.data.data || []);
        if (srs) setAvailableSemanticRouters(srs.data.data || []);
        if (current) {
          setModelRouters((current.data.data.model_routers || []).map(routerOption));
          setSemanticRouters((current.data.data.semantic_routers || []).map(routerOption));
        }
      } catch (error) {
        console.error("Error fetching routers", error);
      }
    };
    fetchRouters();
    return () => {
      cancelled = true;
    };
  }, [id, showModelRouters, showSemanticRouters]);

  const handleChange = (e) => {
    setCatalogue({ ...catalogue, [e.target.name]: e.target.value });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);

    // The picker adds to `llms` the moment an option is chosen; there is no
    // pending "+" step, so what is on screen is exactly what gets saved.
    const desiredLLMs = llms;

    // workspace_id is sent only to choose one (create) or to move (edit).
    const { workspace_id: _storedWorkspaceId, ...attributes } = catalogue;
    const workspaceAttr =
      workspaceId !== null && workspaceId !== undefined && (!id || Number(workspaceId) !== Number(originalWorkspaceId))
        ? { workspace_id: Number(workspaceId) }
        : {};
    const catalogueData = {
      data: {
        type: "Catalogue",
        attributes: { ...attributes, ...workspaceAttr },
      },
    };

    try {
      let catalogueId;
      if (id) {
        await apiClient.patch(`/catalogues/${id}`, catalogueData);
        catalogueId = id;
      } else {
        const response = await apiClient.post("/catalogues", catalogueData);
        catalogueId = response.data.data.id;
      }

      // Now handle LLM additions/removals
      await updateCatalogueLLMs(catalogueId, desiredLLMs);

      if (showModelRouters || showSemanticRouters) {
        const toIds = (list) => list.map((r) => parseInt(r.id, 10));
        await apiClient.put(`/catalogues/${catalogueId}/routers`, {
          ...(showModelRouters && { model_router_ids: toIds(modelRouters) }),
          ...(showSemanticRouters && { semantic_router_ids: toIds(semanticRouters) }),
        });
      }

      markSaved();
      navigate("/admin/catalogs/llms", {
        state: { snackbar: { message: `Catalog ${id ? "updated" : "created"} successfully`, severity: "success" } },
      });
    } catch (error) {
      console.error("Error saving catalog", error);
      const wsError = workspaceFieldError(error);
      if (wsError) {
        setWorkspaceError(wsError);
        setSnackbar({ open: true, message: wsError, severity: "error" });
        setLoading(false);
        return;
      }
      setSnackbar({
        open: true,
        message: "Error saving catalog",
        severity: "error",
      });
      setLoading(false);
    }
  };

  // Takes the desired LLM list explicitly rather than reading `llms` from the
  // closure.
  const updateCatalogueLLMs = async (catalogueId, desiredLLMs) => {
    try {
      const currentLLMs = id
        ? (await apiClient.get(`/catalogues/${catalogueId}/llms`)).data.data
        : [];

      // Remove LLMs that are no longer in the list
      for (let llm of currentLLMs) {
        if (!desiredLLMs.find((l) => l.id === llm.id)) {
          await apiClient.delete(`/catalogues/${catalogueId}/llms/${llm.id}`);
        }
      }

      // Add new LLMs
      for (let llm of desiredLLMs) {
        if (!currentLLMs.find((l) => l.id === llm.id)) {
          await apiClient.post(`/catalogues/${catalogueId}/llms`, {
            data: { id: llm.id, type: "LLM" },
          });
        }
      }

      setSnackbar({
        open: true,
        message: "Model providers updated successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error updating catalog LLMs", error);
      setSnackbar({
        open: true,
        message: "Error updating model providers",
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

  if (notFound) {
    return <NotFound />;
  }

  if (loading) {
    return <CircularProgress />;
  }

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">
          {id ? "Edit model provider catalog" : "Add model provider catalog"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/catalogs/llms"
          color="inherit"
        >
          Back to catalogs
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of model providers and routers that you can assign to specific teams to manage access easily.</Typography>  
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <TextField
            fullWidth
            margin="normal"
            label="Catalog Name"
            name="name"
            value={catalogue.name}
            onChange={handleChange}
            required
          />

          <Box sx={{ mt: 3 }}>
            <RelationshipPicker
              label="Model providers in this catalog"
              itemLabel="model provider"
              value={llms}
              onChange={setLLMs}
              options={availableLLMs}
              getOptionLabel={(llm) => llm.attributes?.name ?? ""}
            />
          </Box>

          {showModelRouters && (
            <Box sx={{ mt: 3 }}>
              <RelationshipPicker
                label="Model routers in this catalog"
                itemLabel="model router"
                value={modelRouters}
                onChange={setModelRouters}
                options={availableModelRouters}
                getOptionLabel={(r) => r.attributes?.name ?? ""}
              />
              <OtherWorkspacesNote permission={P.MODEL_ROUTERS_READ} sx={{ mt: 1 }}>
                Model routers in Workspaces you cannot see are not listed; saving keeps them in this catalog.
              </OtherWorkspacesNote>
            </Box>
          )}

          {showSemanticRouters && (
            <Box sx={{ mt: 3 }}>
              <RelationshipPicker
                label="Semantic routers in this catalog"
                itemLabel="semantic router"
                value={semanticRouters}
                onChange={setSemanticRouters}
                options={availableSemanticRouters}
                getOptionLabel={(r) => r.attributes?.name ?? ""}
              />
              <OtherWorkspacesNote permission={P.SEMANTIC_ROUTERS_READ} sx={{ mt: 1 }}>
                Semantic routers in Workspaces you cannot see are not listed; saving keeps them in this catalog.
              </OtherWorkspacesNote>
            </Box>
          )}

          {/* Workspace (Enterprise): where it is created; on edit, the move control. */}
          <Box sx={{ mt: 3 }}>
            <WorkspaceSelect
              permission={P.CATALOGUES_WRITE}
              value={workspaceId}
              onChange={(ws) => {
                setWorkspaceId(ws);
                setWorkspaceError("");
              }}
              disabled={Boolean(id) && !canMove}
              error={workspaceError}
              helperText={id ? "Moving it to another Workspace needs write in both." : "The Workspace it is created in."}
            />
          </Box>

          <Box mt={3} display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel}>
              Cancel
            </SecondaryOutlineButton>
            <PrimaryButton type="submit" variant="contained" color="primary">
              {id ? "Update catalog" : "Create catalog"}
            </PrimaryButton>
          </Box>
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

export default CatalogueForm;
