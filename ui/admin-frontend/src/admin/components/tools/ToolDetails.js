import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Divider,
  Tooltip,
  List,
  ListItem,
  ListItemText,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const ToolDetails = () => {
  const [tool, setTool] = useState(null);
  const [operations, setOperations] = useState([]);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchToolDetails();
    fetchToolOperations();
  }, [id]);

  const fetchToolDetails = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}`);
      setTool(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching tool details", error);
      setLoading(false);
    }
  };

  const fetchToolOperations = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}/operations`);
      setOperations(response.data.data.operations);
    } catch (error) {
      console.error("Error fetching tool operations", error);
    }
  };

  if (loading) return <CircularProgress />;
  if (!tool) return <Typography>Tool not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Tool Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/tools")}
          color="white"
        >
          Back to Tools
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Tool Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{tool.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{tool.attributes.description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Privacy Score:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{tool.attributes.privacy_score}</FieldValue>
              <Tooltip
                title="Privacy score is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the tool."
                placement="top"
              >
                <HelpOutlineIcon
                  sx={{ ml: 1, fontSize: 20, color: "text.secondary" }}
                />
              </Tooltip>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Tool Type:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>REST</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Authentication Details</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Auth Schema Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{tool.attributes.auth_schema_name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Auth Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {tool.attributes.auth_key ? "*".repeat(20) : "Not set"}
            </FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>OpenAPI Specification</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            <FieldValue>
              {tool.attributes.oas_spec
                ? "OpenAPI Specification is set"
                : "OpenAPI Specification is not set"}
            </FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Operations</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            {operations.length > 0 ? (
              <List sx={{ listStyleType: "decimal", pl: 4 }}>
                {operations.map((operation, index) => (
                  <ListItem key={index} sx={{ display: "list-item" }}>
                    <ListItemText
                      primary={
                        <Typography sx={{ fontFamily: "monospace" }}>
                          {operation}
                        </Typography>
                      }
                    />
                  </ListItem>
                ))}
              </List>
            ) : (
              <FieldValue>No operations set for this tool.</FieldValue>
            )}
          </Grid>
        </Grid>

        <Box mt={4} display="flex" justifyContent="flex-end">
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/tools/edit/${id}`)}
          >
            Edit Tool
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default ToolDetails;
