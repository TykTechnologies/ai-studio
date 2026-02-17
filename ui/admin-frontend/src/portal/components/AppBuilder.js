import React, { useState, useEffect, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  TextField,
  Button,
  Box,
  Chip,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Alert,
  CircularProgress,
  Card,
  CardContent,
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";
import { PrimaryButton } from "../../admin/styles/sharedStyles";

const AppBuilder = () => {
  const [appName, setAppName] = useState("My New App");
  const [description, setDescription] = useState("");
  const [dataSources, setDataSources] = useState([]);
  const [llms, setLLMs] = useState([]);
  const [tools, setTools] = useState([]);
  const [selectedDataSources, setSelectedDataSources] = useState([]);
  const [selectedLLMs, setSelectedLLMs] = useState([]);
  const [selectedTools, setSelectedTools] = useState([]);
  const [pluginResourceTypes, setPluginResourceTypes] = useState([]);
  const [pluginResourceSelections, setPluginResourceSelections] = useState({});
  const [currentPluginResource, setCurrentPluginResource] = useState({});
  const [currentDataSource, setCurrentDataSource] = useState("");
  const [currentLLM, setCurrentLLM] = useState("");
  const [currentTool, setCurrentTool] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [isSubmitted, setIsSubmitted] = useState(false);

  const location = useLocation();
  const navigate = useNavigate();

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

        setIsLoading(false);
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load data. Please try again.");
        setIsLoading(false);
      }
    };

    fetchData();
  }, [location.search]);

  const handleAddDataSource = () => {
    if (
      currentDataSource &&
      !selectedDataSources.some((ds) => ds.id === currentDataSource)
    ) {
      const dataSource = dataSources.find((ds) => ds.id === currentDataSource);
      setSelectedDataSources([...selectedDataSources, dataSource]);
      setCurrentDataSource("");
    }
  };

  const handleAddLLM = () => {
    if (currentLLM && !selectedLLMs.some((llm) => llm.id === currentLLM)) {
      const llm = llms.find((l) => l.id === currentLLM);
      setSelectedLLMs([...selectedLLMs, llm]);
      setCurrentLLM("");
    }
  };

  const handleRemoveDataSource = (id) => {
    setSelectedDataSources(selectedDataSources.filter((ds) => ds.id !== id));
  };

  const handleRemoveLLM = (id) => {
    setSelectedLLMs(selectedLLMs.filter((llm) => llm.id !== id));
  };

  const handleAddTool = () => {
    if (currentTool && !selectedTools.some((tool) => tool.id === currentTool)) {
      const tool = tools.find((t) => t.id === currentTool);
      setSelectedTools([...selectedTools, tool]);
      setCurrentTool("");
    }
  };

  const handleRemoveTool = (id) => {
    setSelectedTools(selectedTools.filter((tool) => tool.id !== id));
  };

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
      setIsSubmitted(true);
    } catch (err) {
      console.error("Error creating app:", err);
      setError("Failed to create app. Please try again.");
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
              <Typography variant="subtitle1" gutterBottom>
                Data Sources (Optional)
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <FormControl fullWidth sx={{ mr: 1 }}>
                  <InputLabel>Select Data Source</InputLabel>
                  <Select
                    value={currentDataSource}
                    onChange={(e) => setCurrentDataSource(e.target.value)}
                    label="Select Data Source"
                  >
                    {dataSources.map((ds) => (
                      <MenuItem key={ds.id} value={ds.id}>
                        {ds.attributes.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Button onClick={handleAddDataSource} variant="outlined">
                  Add
                </Button>
              </Box>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {selectedDataSources.map((ds) => (
                  <Chip
                    key={ds.id}
                    label={ds.attributes.name}
                    onDelete={() => handleRemoveDataSource(ds.id)}
                  />
                ))}
              </Box>
            </Box>
            <Box sx={{ mt: 3, mb: 2 }}>
              <Typography variant="subtitle1" gutterBottom>
                LLMs (Optional)
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <FormControl fullWidth sx={{ mr: 1 }}>
                  <InputLabel>Select LLM</InputLabel>
                  <Select
                    value={currentLLM}
                    onChange={(e) => setCurrentLLM(e.target.value)}
                    label="Select LLM"
                  >
                    {llms.map((llm) => (
                      <MenuItem key={llm.id} value={llm.id}>
                        {llm.attributes.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Button onClick={handleAddLLM} variant="outlined">
                  Add
                </Button>
              </Box>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {selectedLLMs.map((llm) => (
                  <Chip
                    key={llm.id}
                    label={llm.attributes.name}
                    onDelete={() => handleRemoveLLM(llm.id)}
                  />
                ))}
              </Box>
            </Box>
            <Box sx={{ mt: 3, mb: 2 }}>
              <Typography variant="subtitle1" gutterBottom>
                Tools (Optional)
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <FormControl fullWidth sx={{ mr: 1 }}>
                  <InputLabel>Select Tool</InputLabel>
                  <Select
                    value={currentTool}
                    onChange={(e) => setCurrentTool(e.target.value)}
                    label="Select Tool"
                  >
                    {tools.map((tool) => (
                      <MenuItem key={tool.id} value={tool.id}>
                        {tool.attributes.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Button onClick={handleAddTool} variant="outlined">
                  Add
                </Button>
              </Box>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {selectedTools.map((tool) => (
                  <Chip
                    key={tool.id}
                    label={tool.attributes.name}
                    onDelete={() => handleRemoveTool(tool.id)}
                  />
                ))}
              </Box>
            </Box>
            {/* Dynamic Plugin Resource Sections */}
            {pluginResourceTypes.map((rt) => {
              const key = `${rt.plugin_id}:${rt.slug}`;
              const instances = rt.instances || [];
              const selected = pluginResourceSelections[key] || [];
              const currentVal = currentPluginResource[key] || "";

              if (instances.length === 0) return null;

              return (
                <Box key={key} sx={{ mt: 3, mb: 2 }}>
                  <Typography variant="subtitle1" gutterBottom>
                    {rt.name} (Optional)
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                    <FormControl fullWidth sx={{ mr: 1 }}>
                      <InputLabel>Select {rt.name}</InputLabel>
                      <Select
                        value={currentVal}
                        onChange={(e) =>
                          setCurrentPluginResource((prev) => ({
                            ...prev,
                            [key]: e.target.value,
                          }))
                        }
                        label={`Select ${rt.name}`}
                      >
                        {instances.map((inst) => (
                          <MenuItem key={inst.id} value={inst.id}>
                            {inst.name}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    <Button
                      onClick={() => {
                        if (
                          currentVal &&
                          !selected.some((s) => s.id === currentVal)
                        ) {
                          const inst = instances.find(
                            (i) => i.id === currentVal,
                          );
                          if (inst) {
                            setPluginResourceSelections((prev) => ({
                              ...prev,
                              [key]: [...selected, inst],
                            }));
                            setCurrentPluginResource((prev) => ({
                              ...prev,
                              [key]: "",
                            }));
                          }
                        }
                      }}
                      variant="outlined"
                    >
                      Add
                    </Button>
                  </Box>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                    {selected.map((inst) => (
                      <Chip
                        key={inst.id}
                        label={inst.name}
                        onDelete={() =>
                          setPluginResourceSelections((prev) => ({
                            ...prev,
                            [key]: selected.filter((s) => s.id !== inst.id),
                          }))
                        }
                      />
                    ))}
                  </Box>
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
            <PrimaryButton
              type="submit"
              variant="contained"
              color="primary"
              disabled={!isFormValid}
              sx={{ mt: 2 }}
            >
              Create App
            </PrimaryButton>
          </Box>
        </CardContent>
      </Card>
    </Container>
  );
};

export default AppBuilder;
