import React from "react";
import {
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Chip,
  Box,
  Link,
} from "@mui/material";
import { StyledTableHeaderCell, StyledTableCell, StyledTableRow } from "../../styles/sharedStyles";
import { Link as RouterLink } from "react-router-dom";

const getRiskChipColor = (level) => {
  switch (level) {
    case "HIGH":
      return "error";
    case "MEDIUM":
      return "warning";
    case "LOW":
      return "success";
    default:
      return "default";
  }
};

const HighRiskAppsTable = ({ apps, onAppClick }) => {
  if (!apps || apps.length === 0) {
    return (
      <Box sx={{ p: 3, textAlign: "center" }}>
        <Typography color="text.secondary">
          No high-risk applications detected in the selected period.
        </Typography>
      </Box>
    );
  }

  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <StyledTableHeaderCell>Application</StyledTableHeaderCell>
            <StyledTableHeaderCell>Owner</StyledTableHeaderCell>
            <StyledTableHeaderCell align="center">Risk Level</StyledTableHeaderCell>
            <StyledTableHeaderCell>Issues</StyledTableHeaderCell>
            <StyledTableHeaderCell align="right">Risk Score</StyledTableHeaderCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {apps.map((app) => (
            <StyledTableRow
              key={app.app_id}
              sx={{ cursor: "pointer" }}
              onClick={() => onAppClick(app.app_id)}
            >
              <StyledTableCell>
                <Link
                  component={RouterLink}
                  to={`/admin/apps/${app.app_id}`}
                  onClick={(e) => e.stopPropagation()}
                  sx={{ fontWeight: 500 }}
                >
                  {app.app_name}
                </Link>
              </StyledTableCell>
              <StyledTableCell>
                {app.owner_email ? (
                  <Link
                    component={RouterLink}
                    to={`/admin/users/${app.owner_id}`}
                    onClick={(e) => e.stopPropagation()}
                    sx={{ color: "text.secondary" }}
                  >
                    {app.owner_email}
                  </Link>
                ) : (
                  <Typography color="text.secondary">-</Typography>
                )}
              </StyledTableCell>
              <StyledTableCell align="center">
                <Chip
                  label={app.risk_level}
                  color={getRiskChipColor(app.risk_level)}
                  size="small"
                  sx={{ fontWeight: "bold" }}
                />
              </StyledTableCell>
              <StyledTableCell>
                <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                  {app.issues?.map((issue, idx) => (
                    <Chip
                      key={idx}
                      label={issue}
                      size="small"
                      variant="outlined"
                      sx={{ fontSize: "0.75rem" }}
                    />
                  ))}
                </Box>
              </StyledTableCell>
              <StyledTableCell align="right">
                <Typography
                  variant="h6"
                  sx={{
                    fontWeight: "bold",
                    color:
                      app.risk_score >= 50
                        ? "error.main"
                        : app.risk_score >= 20
                        ? "warning.main"
                        : "text.primary",
                  }}
                >
                  {app.risk_score}
                </Typography>
              </StyledTableCell>
            </StyledTableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default HighRiskAppsTable;
