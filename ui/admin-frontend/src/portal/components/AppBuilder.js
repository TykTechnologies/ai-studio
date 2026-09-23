import React, { useState, useEffect, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  TextField,
  Box,
  Alert,
  CircularProgress,
  Card,
  CardContent,
  Grid,
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";
import {
  PrimaryButton,
  SecondaryOutlineButton,
} from "../../admin/styles/sharedStyles";
import { getVendorName } from "../../admin/utils/vendorLogos";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../components/unsaved-changes";
import { CATALOG_TYPES, MCP_KIND_LABELS, typeIcon, typeLabel } from "../utils/catalog";
import {
  AccessPicker,
  RequestedAccessList,
  selectionCount,
} from "./AppAccessPicker";

const jsonApiName = (item) => item?.attributes?.name ?? "";

// Instances of a plugin resource type that an App credential grants access
// to. The server marks each instance (type value with any per-instance
// override applied); a missing flag is treated as granted.
const appGrantedInstances = (resourceType) =>
  (resourceType?.instances || []).filter((inst) => inst.access_granted_via_app !== false);

const pluginGroupKey = (rt) => `plugin:${rt.plugin_id}:${rt.slug}`;

const GROUP_KEYS = {
  LLM: CATALOG_TYPES.LLM,
  DATASOURCE: CATALOG_TYPES.DATASOURCE,
  TOOL: CATALOG_TYPES.TOOL,
  MCP_SERVER: CATALOG_TYPES.MCP_SERVER,
  MODEL_ROUTER: CATALOG_TYPES.MODEL_ROUTER,
};

const ROUTER_HELPER =
  "Called on the unified endpoint as <router>/<model>. The app reaches the router's LLM providers only through the router.";
const TOOL_HELPER =
  "Served by AI Studio. Your app calls them over REST or MCP with its own credential.";
const MCP_HELPER =
  "Served by a Tyk Gateway. Once the app is approved you request a Tyk access key for them on the app page.";

// A second line that only repeats the name ("Anthropic" by Anthropic) is noise.
const withoutEcho = (opt) =>
  opt.secondary && opt.secondary.trim().toLowerCase() === opt.name.trim().toLowerCase()
    ? { ...opt, secondary: "" }
    : opt;

const coreGroup = (type, helperText, options) => ({
  key: type,
  label: typeLabel(type, { plural: true }),
  icon: typeIcon(type),
  helperText,
  options: options.map(withoutEcho),
});

/**
 * One group per asset type an App can be granted, in catalog order. Types
 * with nothing to offer are left out, so a user who can only reach LLM
 * providers sees one tab rather than four empty ones.
 */
const buildGroups = ({ llms, modelRouters = [], dataSources, tools, mcpServers, pluginResourceTypes }) =>
  [
    coreGroup(
      CATALOG_TYPES.LLM,
      null,
      llms.map((llm) => ({
        id: String(llm.id),
        name: jsonApiName(llm),
        secondary: getVendorName(llm.attributes?.vendor) || llm.attributes?.vendor || "",
      })),
    ),
    // Model routers sit in LLM catalogs and stand in for LLM providers.
    coreGroup(
      CATALOG_TYPES.MODEL_ROUTER,
      ROUTER_HELPER,
      modelRouters.map((router) => ({
        id: String(router.id),
        name: router.attributes?.name || "",
        secondary: router.attributes?.short_description || "",
      })),
    ),
    coreGroup(
      CATALOG_TYPES.DATASOURCE,
      null,
      dataSources.map((ds) => ({
        id: String(ds.id),
        name: jsonApiName(ds),
        secondary: ds.attributes?.short_description || "",
      })),
    ),
    coreGroup(
      CATALOG_TYPES.TOOL,
      TOOL_HELPER,
      tools.map((tool) => ({
        id: String(tool.id),
        name: jsonApiName(tool),
        secondary: tool.attributes?.description || "",
      })),
    ),
    coreGroup(
      CATALOG_TYPES.MCP_SERVER,
      MCP_HELPER,
      mcpServers.map((server) => ({
        id: String(server.id),
        name: server.attributes?.name || "",
        secondary:
          server.attributes?.short_description ||
          server.attributes?.description ||
          MCP_KIND_LABELS[server.attributes?.kind] ||
          "",
      })),
    ),
    // Types whose instances are not granted through an App (informational
    // assets, plugin-managed access) offer nothing here.
    ...pluginResourceTypes.map((rt) => ({
      key: pluginGroupKey(rt),
      label: rt.name,
      icon: "puzzle-piece",
      helperText: rt.description || null,
      pluginId: rt.plugin_id,
      slug: rt.slug,
      options: appGrantedInstances(rt).map((inst) => ({
        id: String(inst.id),
        name: inst.name || "",
        secondary: inst.description || "",
      })).map(withoutEcho),
    })),
  ].filter((group) => group.options.length > 0);

/**
 * The ?llm= / ?model_router= / ?datasource= / ?tool= / ?mcp_server= /
 * ?plugin_resource= preselection from a catalog "Build with" link. Only ids the user can
 * actually add are kept; a deep link to anything else is ignored.
 */
const preselectionFrom = (search, groups) => {
  const params = new URLSearchParams(search);
  const selection = {};
  const keep = (key, id) => {
    const group = groups.find((g) => g.key === key);
    if (id && group?.options.some((opt) => opt.id === id)) {
      selection[key] = [...(selection[key] || []), id];
    }
  };

  keep(GROUP_KEYS.LLM, params.get("llm"));
  keep(GROUP_KEYS.MODEL_ROUTER, params.get("model_router"));
  keep(GROUP_KEYS.DATASOURCE, params.get("datasource"));
  keep(GROUP_KEYS.TOOL, params.get("tool"));
  keep(GROUP_KEYS.MCP_SERVER, params.get("mcp_server"));

  // ?plugin_resource=<plugin id>:<slug>:<instance id>, from a plugin
  // resource's catalog page. The instance id may itself contain ":".
  const pluginResource = params.get("plugin_resource");
  if (pluginResource) {
    const [pluginId, slug, ...rest] = pluginResource.split(":");
    keep(`plugin:${pluginId}:${slug}`, rest.join(":"));
  }
  return selection;
};

const toInts = (ids = []) => ids.map((id) => parseInt(id, 10));

const pageSx = {
  px: 3,
  py: 3,
  boxSizing: "border-box",
  width: "100%",
};

const SectionHeading = ({ title, subtitle }) => (
  <Box sx={{ mb: 1.5 }}>
    <Typography variant="headingSmall" component="h2">
      {title}
    </Typography>
    {subtitle && (
      <Typography variant="bodyMediumDefault" color="text.secondary" component="p">
        {subtitle}
      </Typography>
    )}
  </Box>
);

const AppBuilder = () => {
  // No placeholder name: the field is required and a default of "My New App"
  // was being submitted as-is.
  const [appName, setAppName] = useState("");
  const [description, setDescription] = useState("");
  const [groups, setGroups] = useState([]);
  const [selection, setSelection] = useState({});
  const [initialGroupKey, setInitialGroupKey] = useState(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [submittedApp, setSubmittedApp] = useState(null);

  const location = useLocation();
  const navigate = useNavigate();

  // Unsaved-changes tracking. The baseline is taken once the option lists
  // (and any ?llm= / ?datasource= / ?tool= preselection) are on screen, so
  // arriving from a resource page never counts as a change. markSaved()
  // runs before the success screen replaces the form.
  const { markSaved } = useUnsavedForm(
    { appName, description, selection },
    { ready: !isLoading },
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/portal/apps"));

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [dataSourcesResponse, llmsResponse, toolsResponse, pluginResourcesResponse, mcpResponse, routerResponse] =
          await Promise.all([
            pubClient.get("/common/accessible-datasources"),
            pubClient.get("/common/accessible-llms"),
            // Only tools an App can reach (REST or MCP access on). Chat-only
            // tools, the built-in Generative UI tool among them, are left
            // out; the same endpoint without the flag feeds the chat picker.
            pubClient.get("/common/accessible-tools", { params: { app_grantable: true } }),
            pubClient.get("/common/accessible-plugin-resources").catch(() => ({ data: { data: [] } })),
            // Tyk-managed MCP servers (Enterprise) come from the unified
            // catalog; the request fails harmlessly on Community Edition.
            pubClient
              .get("/common/catalog", { params: { type: "mcp_server", page_size: 100 } })
              .catch(() => ({ data: { data: [] } })),
            // Model routers (Enterprise) likewise come from the unified catalog.
            pubClient
              .get("/common/catalog", { params: { type: "model_router", page_size: 100 } })
              .catch(() => ({ data: { data: [] } })),
          ]);
        // Only servers AI Studio brokers (key-backed) belong on an App;
        // OAuth, mTLS and keyless servers are reached directly and the
        // server refuses to bind them.
        const mcpServers = (mcpResponse.data?.data || []).filter(
          (item) => item.attributes?.access_granted_via_app !== false,
        );
        const modelRouters = (routerResponse.data?.data || []).filter(
          (item) => item.attributes?.access_granted_via_app !== false,
        );
        const nextGroups = buildGroups({
          llms: llmsResponse.data || [],
          modelRouters,
          dataSources: dataSourcesResponse.data || [],
          tools: toolsResponse.data || [],
          mcpServers,
          pluginResourceTypes: pluginResourcesResponse.data?.data || [],
        });
        const preselected = preselectionFrom(location.search, nextGroups);

        setGroups(nextGroups);
        setSelection(preselected);
        // Open the picker on the type the user arrived with.
        setInitialGroupKey(nextGroups.find((g) => preselected[g.key])?.key || null);
        setIsLoading(false);
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load data. Please try again.");
        setIsLoading(false);
      }
    };

    fetchData();
  }, [location.search]);

  const toggleItem = (key, id) =>
    setSelection((prev) => {
      const ids = prev[key] || [];
      const next = ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
      const { [key]: _omit, ...rest } = prev;
      return next.length > 0 ? { ...rest, [key]: next } : rest;
    });

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    try {
      const pluginResourcesPayload = groups
        .filter((group) => group.pluginId !== undefined && selection[group.key]?.length)
        .map((group) => ({
          plugin_id: group.pluginId,
          resource_type_slug: group.slug,
          instance_ids: selection[group.key],
        }));
      const mcpServerIds = selection[GROUP_KEYS.MCP_SERVER] || [];
      const modelRouterIds = selection[GROUP_KEYS.MODEL_ROUTER] || [];

      const response = await pubClient.post("/common/apps", {
        name: appName,
        description,
        data_source_ids: toInts(selection[GROUP_KEYS.DATASOURCE]),
        llm_ids: toInts(selection[GROUP_KEYS.LLM]),
        tool_ids: toInts(selection[GROUP_KEYS.TOOL]),
        ...(mcpServerIds.length > 0 && {
          mcp_server_ids: toInts(mcpServerIds),
        }),
        ...(modelRouterIds.length > 0 && {
          model_router_ids: toInts(modelRouterIds),
        }),
        ...(pluginResourcesPayload.length > 0 && {
          plugin_resources: pluginResourcesPayload,
        }),
      });
      markSaved();
      setSubmittedApp({
        id: response?.data?.id ?? null,
        name: appName,
        description,
        selection,
      });
    } catch (err) {
      console.error("Error creating app:", err);
      // The API already explains a privacy refusal -- which resource, which
      // provider, and the two numbers. Discarding that for "Please try again"
      // was actively misleading: nothing about the request has changed, so
      // retrying can never succeed.
      const detail = err?.response?.data?.errors?.[0]?.detail;
      setError(detail || "Failed to create app. Please try again.");
    }
  };

  const requestedCount = selectionCount(selection);

  const isFormValid = useMemo(
    () => appName.trim() !== "" && description.trim() !== "" && requestedCount > 0,
    [appName, description, requestedCount],
  );

  if (isLoading)
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="100vh"
      >
        <CircularProgress />
      </Box>
    );

  if (submittedApp) {
    const hasMCPServers = Boolean(submittedApp.selection[GROUP_KEYS.MCP_SERVER]?.length);
    const hasModelRouters = Boolean(submittedApp.selection[GROUP_KEYS.MODEL_ROUTER]?.length);
    return (
      <Container maxWidth={false} sx={pageSx}>
        <Typography variant="h4" component="h1" gutterBottom>
          App Submitted
        </Typography>
        <Typography variant="body1" paragraph>
          An administrator reviews the request. Once it is approved, the app
          page has the credential to call everything below with.
        </Typography>
        <Card sx={{ maxWidth: 720 }} data-testid="app-submitted-summary">
          <CardContent sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <Box>
              <Typography variant="headingMedium" component="h2">
                {submittedApp.name}
              </Typography>
              <Typography variant="bodyLargeDefault" color="text.secondary" component="p" sx={{ whiteSpace: "pre-line" }}>
                {submittedApp.description}
              </Typography>
            </Box>
            <Box>
              <SectionHeading title="Access requested" />
              <RequestedAccessList groups={groups} selection={submittedApp.selection} />
            </Box>
            {hasMCPServers && (
              <Alert severity="info">
                After approval, request a Tyk access key for the MCP servers
                on the app page.
              </Alert>
            )}
            {hasModelRouters && (
              <Alert severity="info" data-testid="app-submitted-router-note">
                Call the model routers on the unified endpoint with the model
                written as <code>&lt;router&gt;/&lt;model&gt;</code>; the app
                page shows how.
              </Alert>
            )}
          </CardContent>
        </Card>
        <Box sx={{ mt: 3, display: "flex", gap: 2, flexWrap: "wrap" }}>
          {submittedApp.id && (
            <PrimaryButton
              variant="contained"
              color="primary"
              onClick={() => navigate(`/portal/apps/${submittedApp.id}`)}
            >
              Open app
            </PrimaryButton>
          )}
          <SecondaryOutlineButton onClick={() => navigate("/portal/apps")}>
            View your Apps and Credentials
          </SecondaryOutlineButton>
        </Box>
      </Container>
    );
  }

  return (
    <Container maxWidth={false} sx={pageSx}>
      <Typography variant="h4" component="h1" gutterBottom>
        Create New App
      </Typography>
      <Card>
        <CardContent sx={{ p: 3 }}>
          {error && (
            <Alert severity="error" sx={{ mb: 3 }}>
              {error}
            </Alert>
          )}
          <Box component="form" onSubmit={handleSubmit}>
            <Grid container spacing={4}>
              <Grid item xs={12} md={5}>
                <SectionHeading title="Details" />
                <TextField
                  fullWidth
                  label="App Name"
                  value={appName}
                  onChange={(e) => setAppName(e.target.value)}
                  required
                  margin="dense"
                />
                <TextField
                  fullWidth
                  label="Description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  required
                  multiline
                  rows={3}
                  margin="normal"
                  helperText="What the app does. The administrator who approves it reads this."
                />
                <Box sx={{ mt: 3 }}>
                  <SectionHeading
                    title={requestedCount > 0 ? `Access requested (${requestedCount})` : "Access requested"}
                  />
                  <RequestedAccessList
                    groups={groups}
                    selection={selection}
                    onRemove={toggleItem}
                    emptyText="Nothing yet. Add at least one item this app needs from the list."
                  />
                </Box>
              </Grid>
              <Grid item xs={12} md={7}>
                <SectionHeading
                  title="Add access"
                  subtitle="Pick a type, then add what the app needs. You can mix types."
                />
                {groups.length > 0 ? (
                  <AccessPicker
                    groups={groups}
                    selection={selection}
                    onToggle={toggleItem}
                    initialGroupKey={initialGroupKey}
                  />
                ) : (
                  <Alert severity="info">
                    Nothing is available to add to an app yet. Ask an
                    administrator to add you to a team with a catalog.
                  </Alert>
                )}
                <Typography
                  variant="bodySmallDefault"
                  color="text.secondary"
                  component="p"
                  sx={{ mt: 1.5 }}
                >
                  Some combinations are refused to protect data: nothing on
                  an app can be more sensitive than its LLM providers are
                  cleared for.
                </Typography>
              </Grid>
            </Grid>
            <Box sx={{ mt: 4, display: "flex", gap: 2, alignItems: "center", flexWrap: "wrap" }}>
              <SecondaryOutlineButton onClick={handleCancel}>
                Cancel
              </SecondaryOutlineButton>
              <PrimaryButton
                type="submit"
                variant="contained"
                color="primary"
                disabled={!isFormValid}
              >
                Create App
              </PrimaryButton>
              {!isFormValid && (
                <Typography variant="bodySmallDefault" color="text.secondary">
                  Add a name, a description and at least one item to continue.
                </Typography>
              )}
            </Box>
          </Box>
        </CardContent>
      </Card>
    </Container>
  );
};

export default AppBuilder;
