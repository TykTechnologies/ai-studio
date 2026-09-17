import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import Section from "../common/Section";
import { CatalogueTeamsSection } from "../common/UsedBySection";
import { AccessMethodChips } from "../tools/ToolAccessMethods";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Chip,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Divider,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
} from "../../styles/sharedStyles";

const ToolCatalogueDetails = () => {
  const [catalogue, setCatalogue] = useState(null);
  // null = the API did not send the list (integration off, or no permission).
  const [mcpServers, setMcpServers] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchToolCatalogueDetails();
  }, [id]);

  const fetchToolCatalogueDetails = async () => {
    try {
      const response = await apiClient.get(`/tool-catalogues/${id}`);
      setCatalogue(response.data?.data);
      setMcpServers(response.data?.mcp_servers ?? null);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching tool catalogue details", error);
      setError("Failed to load tool catalogue details");
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!catalogue) return <Typography>Tool catalogue not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Tool catalog details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/catalogs/tools")}
          color="inherit"
        >
          Back to catalogs
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <Section title="Catalog Description">
          <Grid container spacing={2}>
            <Grid item xs={3}>
              <FieldLabel>Name:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>{catalogue.attributes.name}</FieldValue>
            </Grid>
            <Grid item xs={3}>
              <FieldLabel>Short Description:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>{catalogue.attributes.short_description}</FieldValue>
            </Grid>
            <Grid item xs={3}>
              <FieldLabel>Long Description:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>{catalogue.attributes.long_description}</FieldValue>
            </Grid>
            <Grid item xs={3}>
              <FieldLabel>Icon:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>{catalogue.attributes.icon}</FieldValue>
            </Grid>
          </Grid>
        </Section>

        <Section title="Tools">
          <List>
            {catalogue.attributes.tools &&
            catalogue.attributes.tools.length > 0 ? (
              catalogue.attributes.tools.map((tool) => (
                <React.Fragment key={tool.id}>
                  <ListItem
                    secondaryAction={<AccessMethodChips attributes={tool.attributes} />}
                  >
                    <ListItemText
                      primary={tool.attributes.name}
                      secondary={
                        tool.attributes.app_grantable === false
                          ? `${tool.attributes.description || ""} Chat only: teams get this tool in chat, but it is not shown in the portal.`.trim()
                          : tool.attributes.description
                      }
                    />
                  </ListItem>
                  <Divider />
                </React.Fragment>
              ))
            ) : (
              <ListItem>
                <ListItemText primary="No tools in this catalog" />
              </ListItem>
            )}
          </List>
        </Section>

        {/* MCP servers behind a Tyk Gateway are published into tool catalogs
            too. The API sends the list only when the integration is on and
            the caller may read MCP servers. */}
        {Array.isArray(mcpServers) && (
          <Section title="MCP servers">
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Served by a Tyk Gateway and published in this catalog. Manage them under MCP servers.
            </Typography>
            <List data-testid="catalogue-mcp-servers">
              {mcpServers.length > 0 ? (
                mcpServers.map((server) => (
                  <React.Fragment key={server.id}>
                    <ListItem
                      disablePadding
                      secondaryAction={
                        <Chip size="small" variant="outlined" label={server.published ? "Published" : "Unpublished"} />
                      }
                    >
                      <ListItemButton onClick={() => navigate(`/admin/mcp-servers/${server.id}`)}>
                        <ListItemText
                          primary={server.name}
                          secondary={server.kind === "rest_to_mcp" ? "REST API to MCP" : "Remote MCP"}
                        />
                      </ListItemButton>
                    </ListItem>
                    <Divider />
                  </React.Fragment>
                ))
              ) : (
                <ListItem>
                  <ListItemText primary="No MCP servers in this catalog" />
                </ListItem>
              )}
            </List>
          </Section>
        )}

        <Section title="Tags">
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
            {catalogue.attributes.tags && catalogue.attributes.tags.length > 0 ? (
              catalogue.attributes.tags.map((tag) => (
                <Chip key={tag.id} label={tag.attributes.name} />
              ))
            ) : (
              <Typography>No tags for this catalog</Typography>
            )}
          </Box>
        </Section>

        <CatalogueTeamsSection resourcePath="tool-catalogues" id={id} />

        <Box
          mt={4}
          display="flex"
          justifyContent="flex-end"
          alignItems="center"
        >
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/catalogs/tools/edit/${id}`)}
          >
            Edit catalog
          </PrimaryButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default ToolCatalogueDetails;
