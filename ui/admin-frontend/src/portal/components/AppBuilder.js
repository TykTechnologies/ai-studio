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
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";
import {
  PrimaryButton,
  SecondaryOutlineButton,
} from "../../admin/styles/sharedStyles";
import RelationshipPicker from "../../admin/components/common/relationship-picker";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../components/unsaved-changes";

const jsonApiName = (item) => item?.attributes?.name ?? "";

const AppBuilder = () => {
  // No placeholder name: the field is required and a default of "My New App"
  // was being submitted as-is.
  const [appName, setAppName] = useState("");
  const [description, setDescription] = useState("");
  const [dataSources, setDataSources] = useState([]);
  const [llms, setLLMs] = useState([]);
  const [tools, setTools] = useState([]);
  const [selectedDataSources, setSelectedDataSources] = useState([]);
  const [selectedLLMs, setSelectedLLMs] = useState([]);
  const [selectedTools, setSelectedTools] = useState([]);
  const [pluginResourceTypes, setPluginResourceTypes] = useState([]);
  const [pluginResourceSelections, setPluginResourceSelections] = useState({});
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [isSubmitted, setIsSubmitted] = useState(false);

  const location = useLocation();
  const navigate = useNavigate();

  // Unsaved-changes tracking. The baseline is taken once the option lists
  // (and any ?llm= / ?datasource= / ?tool= preselection) are on screen, so
  // arriving from a resource page never counts as a change. markSaved()
  // runs before the success screen replaces the form.
  const { markSaved } = useUnsavedForm(
    {
      appName,
      description,
      dataSourceIds: selectedDataSources.map((ds) => String(ds.id)),
      llmIds: selectedLLMs.map((llm) => String(llm.id)),
      toolIds: selectedTools.map((tool) => String(tool.id)),
      pluginResourceIds: Object.fromEntries(
        Object.entries(pluginResourceSelections).map(([key, items]) => [
          key,
          items.map((item) => String(item.id)),
        ]),
      ),
    },
    { ready: !isLoading },
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/portal/apps"));

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [dataSourcesResponse, llmsResponse, toolsResponse, pluginResourcesResponse] =
          await Promise.all([
            pubClient.get("/common/accessible-datasources"),
            pubClient.get("/common/accessible-llms"),
            pubClient.get("/common/accessible-tools"),
            pubClient.get("/common/accessible-plugin-resources").catch(() => ({ data: { data: [] } })),
          ]);
        setDataSources(dataSourcesResponse.data);
        setLLMs(llmsResponse.data);
        setTools(toolsResponse.data);
        setPluginResourceTypes(pluginResourcesResponse.data?.data || []);

        // Parse query parameters
        const params = new URLSearchParams(location.search);
        const dataSourceId = params.get("datasource");
        const llmId = params.get("llm");
        const toolId = params.get("tool");

        if (dataSourceId) {
          const dataSource = dataSourcesResponse.data.find(
            (ds) => ds.id === dataSourceId,
          );
          if (dataSource) setSelectedDataSources([dataSource]);
        }

        if (llmId) {
          const llm = llmsResponse.data.find((l) => l.id === llmId);
          if (llm) setSelectedLLMs([llm]);
        }

        if (toolId) {
          const tool = toolsResponse.data.find((t) => t.id === toolId);
          if (tool) setSelectedTools([tool]);
        }

        // ?plugin_resource=<plugin id>:<slug>:<instance id>, from a plugin
        // resource's catalog page. The instance id may itself contain ":".
        const pluginResource = params.get("plugin_resource");
        if (pluginResource) {
          const [pluginId, slug, ...rest] = pluginResource.split(":");
          const instanceId = rest.join(":");
          const key = `${pluginId}:${slug}`;
          const resourceType = (pluginResourcesResponse.data?.data || []).find(
            (t) => `${t.plugin_id}:${t.slug}` === key,
          );
          const instance = resourceType?.instances?.find((inst) => String(inst.id) === instanceId);
          if (instance) setPluginResourceSelections({ [key]: [instance] });
        }

        setIsLoading(false);
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load data. Please try again.");
        setIsLoading(false);
      }
    };

    fetchData();
  }, [location.search]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    try {
      // Build plugin resource selections
      const pluginResourcesPayload = Object.entries(pluginResourceSelections)
        .filter(([, items]) => items.length > 0)
        .map(([key, items]) => {
          const rt = pluginResourceTypes.find(
            (t) => `${t.plugin_id}:${t.slug}` === key,
          );
          return {
            plugin_id: rt ? rt.plugin_id : 0,
            resource_type_slug: rt ? rt.slug : "",
            instance_ids: items.map((item) => item.id),
          };
        });

      const response = await pubClient.post("/common/apps", {
        name: appName,
        description,
        data_source_ids: selectedDataSources.map((ds) => parseInt(ds.id, 10)),
        llm_ids: selectedLLMs.map((llm) => parseInt(llm.id, 10)),
        tool_ids: selectedTools.map((tool) => parseInt(tool.id, 10)),
        ...(pluginResourcesPayload.length > 0 && {
          plugin_resources: pluginResourcesPayload,
        }),
      });
      markSaved();
      setIsSubmitted(true);
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

  const hasPluginResourceSelections = useMemo(() => {
    return Object.values(pluginResourceSelections).some(
      (items) => items.length > 0,
    );
  }, [pluginResourceSelections]);

  const isFormValid = useMemo(() => {
    return (
      appName.trim() !== "" &&
      description.trim() !== "" &&
      (selectedDataSources.length > 0 ||
        selectedLLMs.length > 0 ||
        selectedTools.length > 0 ||
        hasPluginResourceSelections)
    );
  }, [appName, description, selectedDataSources, selectedLLMs, selectedTools, hasPluginResourceSelections]);

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

  if (isSubmitted) {
    return (
      <Container maxWidth="md">
        <Typography variant="h4" component="h1" gutterBottom>
          App Submitted
        </Typography>
        <Typography variant="body1" paragraph>
          Your app has been successfully submitted for approval.
        </Typography>
        <PrimaryButton
          variant="contained"
          color="primary"
          onClick={() => navigate("/portal/apps")}
        >
          View your Apps and Credentials
        </PrimaryButton>
      </Container>
    );
  }

  return (
    <Container
      maxWidth={false}
      sx={{
        px: 3,
        py: 3,
        boxSizing: "border-box",
        width: "100%",
      }}
    >
      <Typography variant="h4" component="h1" gutterBottom>
        Create New App
      </Typography>
      <Card>
        <CardContent>
          {error && (
            <Alert severity="error" sx={{ mt: 2, mb: 2 }}>
              {error}
            </Alert>
          )}
          <Box component="form" onSubmit={handleSubmit} sx={{ mt: 3 }}>
            <TextField
              fullWidth
              label="App Name"
              value={appName}
              onChange={(e) => setAppName(e.target.value)}
              required
              margin="normal"
            />
            <TextField
              fullWidth
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              required
              multiline
              rows={4}
              margin="normal"
            />
            <Box sx={{ mt: 3, mb: 2 }}>
              <RelationshipPicker
                label="Data sources (optional)"
                itemLabel="data source"
                value={selectedDataSources}
                onChange={setSelectedDataSources}
                options={dataSources}
                getOptionLabel={jsonApiName}
              />
            </Box>
            <Box sx={{ mt: 3, mb: 2 }}>
              <RelationshipPicker
                label="LLM providers (optional)"
                itemLabel="LLM provider"
                value={selectedLLMs}
                onChange={setSelectedLLMs}
                options={llms}
                getOptionLabel={jsonApiName}
              />
            </Box>
            <Box sx={{ mt: 3, mb: 2 }}>
              <RelationshipPicker
                label="Tools (optional)"
                itemLabel="tool"
                value={selectedTools}
                onChange={setSelectedTools}
                options={tools}
                getOptionLabel={jsonApiName}
              />
            </Box>
            {/* Dynamic Plugin Resource Sections: one picker per resource
                type; selections are the full instance objects. */}
            {pluginResourceTypes.map((rt) => {
              const key = `${rt.plugin_id}:${rt.slug}`;
              const instances = rt.instances || [];
              const selected = pluginResourceSelections[key] || [];

              if (instances.length === 0) return null;

              return (
                <Box key={key} sx={{ mt: 3, mb: 2 }}>
                  <RelationshipPicker
                    label={`${rt.name} (Optional)`}
                    itemLabel={rt.name.toLowerCase()}
                    value={selected}
                    onChange={(items) =>
                      setPluginResourceSelections((prev) => ({
                        ...prev,
                        [key]: items,
                      }))
                    }
                    options={instances}
                    getOptionLabel={(inst) => inst?.name ?? ""}
                  />
                </Box>
              );
            })}
            <Alert severity="info" sx={{ mt: 2, mb: 2 }}>
              You must select at least one resource for your app. You can add
              multiple of each if needed. Not all resources are allowed to be
              used together in an app due to data security - please ensure the
              resources you select are compatible. Once your App has been
              approved, you will be able to start building your app using the
              credentials provided.
            </Alert>
            <Box sx={{ mt: 2, display: "flex", gap: 2 }}>
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
            </Box>
          </Box>
        </CardContent>
      </Card>
    </Container>
  );
};

export default AppBuilder;
