import React, { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  Typography,
  Box,
  Grid,
  Table,
  TableBody,
  TableHead,
  TableRow,
  TableContainer,
  CircularProgress,
  Alert,
  Chip,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Button,
} from "@mui/material";
import RuleOutlinedIcon from "@mui/icons-material/RuleOutlined";
import {
  TitleBox,
  ContentBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import EnterpriseFeatureBadge from "../components/common/EnterpriseFeatureBadge";
import {
  isGovernedMetadataAvailable,
  getMetadataComplianceReport,
  getMetadataObjectTypes,
} from "../services/governedMetadataService";

const STATUSES = [
  { value: "missing", label: "Missing", color: "error" },
  { value: "invalid", label: "Invalid", color: "error" },
  { value: "expired", label: "Expired", color: "warning" },
  { value: "warnings", label: "Warnings", color: "warning" },
  { value: "valid", label: "Valid", color: "success" },
];

const statusMeta = (status) => STATUSES.find((s) => s.value === status) || { label: status, color: "default" };

export const objectEditPath = (objectType, objectId) => {
  switch (objectType) {
    case "llm":
      return `/admin/llms/edit/${objectId}`;
    case "tool":
      return `/admin/tools/edit/${objectId}`;
    case "datasource":
      return `/admin/datasources/edit/${objectId}`;
    default:
      return null;
  }
};

export const objectViewPath = (objectType, objectId) => {
  switch (objectType) {
    case "llm":
      return `/admin/llms/${objectId}`;
    case "tool":
      return `/admin/tools/${objectId}`;
    case "datasource":
      return `/admin/datasources/${objectId}`;
    default:
      return null;
  }
};

const MetadataCompliance = () => {
  const [available, setAvailable] = useState(null);
  const [objectTypes, setObjectTypes] = useState([]);
  const [objectType, setObjectType] = useState("");
  const [status, setStatus] = useState("");
  const [report, setReport] = useState({ entries: [], counts: {} });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const fetchReport = useCallback(async () => {
    try {
      setLoading(true);
      const params = {};
      if (objectType) params.object_type = objectType;
      if (status) params.status = status;
      const data = await getMetadataComplianceReport(params);
      setReport({ entries: data?.entries || [], counts: data?.counts || {} });
      setError("");
    } catch (err) {
      setError("Failed to load the metadata compliance report");
    } finally {
      setLoading(false);
    }
  }, [objectType, status]);

  useEffect(() => {
    (async () => {
      const ok = await isGovernedMetadataAvailable();
      setAvailable(ok);
      if (ok) {
        try {
          setObjectTypes(await getMetadataObjectTypes());
        } catch (err) {
          // object types are only used for labels
        }
      } else {
        setLoading(false);
      }
    })();
  }, []);

  useEffect(() => {
    if (available) fetchReport();
  }, [available, fetchReport]);

  const typeLabel = (slug) => objectTypes.find((t) => t.slug === slug)?.label || slug;

  return (
    <>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <RuleOutlinedIcon />
          <Typography variant="headingXLarge">Metadata Compliance</Typography>
          <Chip label="Enterprise" size="small" color="primary" />
        </Box>
      </TitleBox>

      <ContentBox sx={{ pt: 0 }}>
        {available === false && (
          <EnterpriseFeatureBadge
            feature="Governed Metadata"
            description="See which LLMs, tools and data sources are missing required governance metadata with the Enterprise Edition."
          />
        )}
        {available && (
          <>
            <Grid container spacing={2} sx={{ mb: 3 }}>
              {STATUSES.map((s) => (
                <Grid item xs={6} sm={4} md={2.4} key={s.value}>
                  <StyledPaper sx={{ p: 2, textAlign: "center" }}>
                    <Typography variant="h4">{report.counts[s.value] || 0}</Typography>
                    <Chip label={s.label} size="small" color={s.color} variant="outlined" />
                  </StyledPaper>
                </Grid>
              ))}
            </Grid>

            <Box sx={{ display: "flex", gap: 2, mb: 2, flexWrap: "wrap" }}>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel id="compliance-object-type-label">Object type</InputLabel>
                <Select
                  labelId="compliance-object-type-label"
                  label="Object type"
                  value={objectType}
                  onChange={(e) => setObjectType(e.target.value)}
                >
                  <MenuItem value="">All</MenuItem>
                  {objectTypes
                    .filter((t) => t.source === "builtin")
                    .map((t) => (
                      <MenuItem key={t.slug} value={t.slug}>
                        {t.label}
                      </MenuItem>
                    ))}
                </Select>
              </FormControl>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel id="compliance-status-label">Status</InputLabel>
                <Select
                  labelId="compliance-status-label"
                  label="Status"
                  value={status}
                  onChange={(e) => setStatus(e.target.value)}
                >
                  <MenuItem value="">All</MenuItem>
                  {STATUSES.map((s) => (
                    <MenuItem key={s.value} value={s.value}>
                      {s.label}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Box>

            {loading && <CircularProgress />}
            {error && <Alert severity="error">{error}</Alert>}
            {!loading && !error && (
              <TableContainer component={StyledPaper}>
                <Table>
                  <TableHead>
                    <TableRow>
                      <StyledTableHeaderCell>Object</StyledTableHeaderCell>
                      <StyledTableHeaderCell>Type</StyledTableHeaderCell>
                      <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                      <StyledTableHeaderCell>Issues</StyledTableHeaderCell>
                      <StyledTableHeaderCell>Last validated</StyledTableHeaderCell>
                      <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {report.entries.length === 0 ? (
                      <TableRow>
                        <StyledTableCell colSpan={6} align="center">
                          <Typography color="text.secondary" sx={{ py: 3 }}>
                            Nothing to report. Either every object is compliant or no schema applies.
                          </Typography>
                        </StyledTableCell>
                      </TableRow>
                    ) : (
                      report.entries.map((entry) => {
                        const meta = statusMeta(entry.status);
                        const viewPath = objectViewPath(entry.object_type, entry.object_id);
                        const editPath = objectEditPath(entry.object_type, entry.object_id);
                        return (
                          <StyledTableRow key={`${entry.object_type}:${entry.object_id}`}>
                            <StyledTableCell>
                              {viewPath ? (
                                <Link to={viewPath}>{entry.object_name || entry.object_id}</Link>
                              ) : (
                                entry.object_name || entry.object_id
                              )}
                            </StyledTableCell>
                            <StyledTableCell>{typeLabel(entry.object_type)}</StyledTableCell>
                            <StyledTableCell>
                              <Chip label={meta.label} size="small" color={meta.color} />
                            </StyledTableCell>
                            <StyledTableCell>
                              {(entry.issues || []).length === 0 ? (
                                "—"
                              ) : (
                                <Box component="ul" sx={{ m: 0, pl: 2 }}>
                                  {entry.issues.map((issue, i) => (
                                    <li key={`${issue.field}-${i}`}>
                                      <Typography variant="body2">
                                        <code>{issue.field}</code>: {issue.message}
                                      </Typography>
                                    </li>
                                  ))}
                                </Box>
                              )}
                            </StyledTableCell>
                            <StyledTableCell>
                              {entry.last_validated_at ? new Date(entry.last_validated_at).toLocaleString() : "Never"}
                            </StyledTableCell>
                            <StyledTableCell>
                              {editPath && (
                                <Button component={Link} to={editPath} size="small">
                                  Edit
                                </Button>
                              )}
                            </StyledTableCell>
                          </StyledTableRow>
                        );
                      })
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </>
        )}
      </ContentBox>
    </>
  );
};

export default MetadataCompliance;
