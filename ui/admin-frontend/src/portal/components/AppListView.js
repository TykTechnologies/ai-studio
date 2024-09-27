// src/portal/components/AppListView.js
import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Button,
  CircularProgress,
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";

const AppListView = () => {
  const [apps, setApps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchApps = async () => {
      try {
        const response = await pubClient.get("/common/apps");
        setApps(response.data.data);
        setLoading(false);
      } catch (err) {
        console.error("Error fetching apps:", err);
        setError("Failed to fetch apps. Please try again later.");
        setLoading(false);
      }
    };

    fetchApps();
  }, []);

  const handleAppClick = (appId) => {
    navigate(`/portal/apps/${appId}`);
  };

  if (loading) {
    return (
      <Container sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Container>
    );
  }

  if (error) {
    return (
      <Container>
        <Typography color="error" sx={{ textAlign: "center", mt: 4 }}>
          {error}
        </Typography>
      </Container>
    );
  }

  return (
    <Container maxWidth="lg">
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        My Apps
      </Typography>
      <TableContainer component={Paper}>
        <Table sx={{ minWidth: 650 }} aria-label="apps table">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Data Sources</TableCell>
              <TableCell>LLMs</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {apps.map((app) => (
              <TableRow
                key={app.id}
                sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
              >
                <TableCell component="th" scope="row">
                  {app.attributes.name}
                </TableCell>
                <TableCell>{app.attributes.description}</TableCell>
                <TableCell>{app.attributes.datasource_ids.length}</TableCell>
                <TableCell>{app.attributes.llm_ids.length}</TableCell>
                <TableCell>
                  <Button
                    variant="contained"
                    onClick={() => handleAppClick(app.id)}
                  >
                    View Details
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Container>
  );
};

export default AppListView;
