import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import apiClient from "../utils/apiClient";
import { TitleBox, ContentBox } from "../styles/sharedStyles";

// The licence as the server reports it (GET /license): SKUs, expiry,
// features, each limit with its usage, and the namespaces holding a slot.
// Everything shown comes from the report; the page knows no limit names.

const errorDetail = (err, fallback) => err?.response?.data?.errors?.[0]?.detail || err?.message || fallback;

const formatDate = (value) => (value ? new Date(value).toLocaleString() : "");

const usageText = (limit) => (limit.unlimited ? `${limit.used} of unlimited` : `${limit.used} of ${limit.max}`);

const Section = ({ title, children }) => (
  <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
    <Typography variant="headingMedium" component="h2" sx={{ mb: 1.5 }}>
      {title}
    </Typography>
    {children}
  </Paper>
);

const License = () => {
  const [report, setReport] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [releaseError, setReleaseError] = useState(null);
  const [releasing, setReleasing] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiClient.get("/license");
      setReport(res.data?.data || null);
    } catch (err) {
      setError(errorDetail(err, "Failed to load the licence."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const release = async (namespace) => {
    setReleasing(namespace);
    setReleaseError(null);
    try {
      await apiClient.delete(`/license/namespaces/${encodeURIComponent(namespace)}`);
      await load();
    } catch (err) {
      setReleaseError(errorDetail(err, `Failed to release the slot of ${namespace}.`));
    } finally {
      setReleasing(null);
    }
  };

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">License</Typography>
      </TitleBox>
      <ContentBox>
        {loading && !report && <CircularProgress aria-label="Loading licence" />}
        {error && <Alert severity="error">{error}</Alert>}
        {report && (
          <>
            <Section title="Licence">
              {!report.valid && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  No valid licence is loaded.
                </Alert>
              )}
              <Typography variant="body1" component="p">
                Products: {(report.skus || []).join(", ") || "none"}
              </Typography>
              {report.expires_at && (
                <Typography variant="body1" component="p" data-testid="license-expiry">
                  Expires {formatDate(report.expires_at)} ({report.days_left} days left)
                </Typography>
              )}
            </Section>

            <Section title="Limits">
              <Table size="small" aria-label="Licence limits">
                <TableHead>
                  <TableRow>
                    <TableCell>Limit</TableCell>
                    <TableCell>Usage</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {(report.limits || []).map((limit) => (
                    <TableRow key={limit.key} data-testid={`license-limit-${limit.key}`}>
                      <TableCell>{limit.label}</TableCell>
                      <TableCell>
                        {usageText(limit)}
                        {!limit.unlimited && limit.used >= limit.max && (
                          <Chip label="At limit" size="small" color="warning" sx={{ ml: 1 }} />
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Section>

            <Section title="Features">
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {(report.features || []).length === 0 && (
                  <Typography variant="body2">No features are licensed.</Typography>
                )}
                {(report.features || []).map((feature) => (
                  <Chip
                    key={feature.key}
                    label={feature.label}
                    color={feature.enabled ? "success" : "default"}
                    variant={feature.enabled ? "filled" : "outlined"}
                    data-testid={`license-feature-${feature.key}`}
                  />
                ))}
              </Box>
            </Section>

            <Section title="Licensed namespaces">
              <Typography variant="body2" component="p" sx={{ mb: 1.5 }}>
                Each edge namespace other than the default one holds a licensed slot. A slot with no connected edge
                gateway is freed automatically after a day; releasing it frees it now. A freed slot goes to the
                longest-waiting namespace, whose edge gateways are configured straight away.
              </Typography>
              {releaseError && (
                <Alert severity="error" sx={{ mb: 2 }} onClose={() => setReleaseError(null)}>
                  {releaseError}
                </Alert>
              )}
              {(report.namespaces || []).length === 0 ? (
                <Typography variant="body2">No namespace holds a slot.</Typography>
              ) : (
                <Table size="small" aria-label="Licensed namespaces">
                  <TableHead>
                    <TableRow>
                      <TableCell>Namespace</TableCell>
                      <TableCell>Claimed</TableCell>
                      <TableCell>Connected edges</TableCell>
                      <TableCell>Idle</TableCell>
                      <TableCell />
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {report.namespaces.map((ns) => {
                      const busy = ns.connected_edges > 0;
                      const button = (
                        <span>
                          <Button
                            size="small"
                            variant="outlined"
                            disabled={busy || releasing !== null}
                            onClick={() => release(ns.namespace)}
                          >
                            Release slot
                          </Button>
                        </span>
                      );
                      return (
                        <TableRow key={ns.namespace}>
                          <TableCell>{ns.namespace}</TableCell>
                          <TableCell>{formatDate(ns.claimed_at)}</TableCell>
                          <TableCell>{ns.connected_edges}</TableCell>
                          <TableCell data-testid={`license-idle-${ns.namespace}`}>
                            {ns.frees_at ? `Since ${formatDate(ns.idle_since)}; frees ${formatDate(ns.frees_at)}` : ""}
                          </TableCell>
                          <TableCell align="right">
                            {busy ? (
                              <Tooltip title="Disconnect the namespace's edge gateways before releasing its slot.">
                                {button}
                              </Tooltip>
                            ) : (
                              button
                            )}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              )}
            </Section>

            {(report.waiting || []).length > 0 && (
              <Section title="Waiting namespaces">
                <Typography variant="body2" component="p" sx={{ mb: 1.5 }}>
                  These namespaces have no licensed slot, so their edge gateways get no configuration. They are
                  admitted in this order as slots free.
                </Typography>
                <Table size="small" aria-label="Waiting namespaces">
                  <TableHead>
                    <TableRow>
                      <TableCell>Position</TableCell>
                      <TableCell>Namespace</TableCell>
                      <TableCell>Waiting since</TableCell>
                      <TableCell>Connected edges</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {report.waiting.map((w) => (
                      <TableRow key={w.namespace} data-testid={`license-waiting-${w.namespace}`}>
                        <TableCell>{w.position}</TableCell>
                        <TableCell>{w.namespace}</TableCell>
                        <TableCell>{formatDate(w.waiting_since)}</TableCell>
                        <TableCell>{w.connected_edges}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Section>
            )}
          </>
        )}
      </ContentBox>
    </>
  );
};

export default License;
