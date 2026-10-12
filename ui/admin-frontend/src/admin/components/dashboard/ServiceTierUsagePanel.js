import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  FormControl,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import apiClient from "../../utils/apiClient";

const ALL = "__all__";
// The API's filter value for untiered traffic (service_tier "").
const STANDARD = "none";

// Labels for the tiers vendors report today (Anthropic batch results:
// batch, standard, priority; OpenAI: flex, priority, scale). The set is open,
// so any other value is shown as-is. Untiered rows ("") are list-price traffic,
// including OpenAI's "default"/"auto" (the report maps them to ""), and rows
// recorded before tiers were; they are kept apart from an explicit "standard".
const TIER_LABELS = {
  batch: "Batch",
  standard: "Standard",
  priority: "Priority",
  flex: "Flex",
  scale: "Scale",
};
export const UNTIERED_LABEL = "Untiered (list price)";

export const tierLabel = (tier) => (tier ? TIER_LABELS[tier] || tier : UNTIERED_LABEL);

const formatAmount = (value, currency) => {
  const n = Number(value || 0).toFixed(4);
  return !currency || currency === "USD" ? `$${n}` : `${n} ${currency}`;
};

// Costs in different currencies are listed side by side, never summed.
const formatCosts = (byCurrency) => {
  const entries = Object.entries(byCurrency || {});
  if (entries.length === 0) return "-";
  return entries
    .sort(([a], [b]) => (a === "USD" ? -1 : b === "USD" ? 1 : a.localeCompare(b)))
    .map(([currency, value]) => formatAmount(value, currency))
    .join(" + ");
};

const filterKey = (tier) => (tier ? tier : STANDARD);

// ServiceTierUsagePanel splits the dashboard's range by the service tier the
// vendor billed at (batch, priority, flex; untiered traffic is listed apart),
// from /analytics/service-tier-usage. Costs are already at each tier's price.
// It stays hidden until some traffic ran at a non-standard tier.
const ServiceTierUsagePanel = ({ startDate, endDate }) => {
  const [usage, setUsage] = useState(null);
  const [error, setError] = useState("");
  const [tier, setTier] = useState(ALL);

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/analytics/service-tier-usage", { params: { start_date: startDate, end_date: endDate } })
      .then((response) => {
        if (cancelled) return;
        setUsage(response.data);
        setError("");
      })
      .catch(() => {
        if (!cancelled) setError("Failed to load service tier usage");
      });
    return () => {
      cancelled = true;
    };
  }, [startDate, endDate]);

  if (error) {
    return (
      <Paper elevation={3} sx={{ p: 2, mt: 3 }} data-testid="service-tier-usage">
        <Alert severity="error">{error}</Alert>
      </Paper>
    );
  }
  const byTier = usage?.by_tier || [];
  if (!byTier.some((t) => t.service_tier)) return null;

  const models = (usage.by_model || []).filter((m) => tier === ALL || filterKey(m.service_tier) === tier);

  return (
    <Paper elevation={3} sx={{ p: 2, mt: 3 }} data-testid="service-tier-usage">
      <Typography variant="h6" gutterBottom>
        Service Tiers
      </Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        Usage by the tier the vendor billed it at. Costs are at each tier's price, for example batch
        at half the list price.
      </Typography>
      <Box sx={{ overflowX: "auto", mb: 3 }}>
        <Table size="small" data-testid="service-tier-summary">
          <TableHead>
            <TableRow>
              <TableCell>Tier</TableCell>
              <TableCell align="right">Requests</TableCell>
              <TableCell align="right">Tokens</TableCell>
              <TableCell align="right">Cost</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {byTier.map((t) => (
              <TableRow key={filterKey(t.service_tier)}>
                <TableCell>{tierLabel(t.service_tier)}</TableCell>
                <TableCell align="right">{t.requests.toLocaleString()}</TableCell>
                <TableCell align="right">{t.total_tokens.toLocaleString()}</TableCell>
                <TableCell align="right">{formatCosts(t.cost_by_currency)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>

      <FormControl size="small" sx={{ minWidth: 200, mb: 1 }}>
        <InputLabel id="service-tier-filter-label">Service tier</InputLabel>
        <Select
          labelId="service-tier-filter-label"
          label="Service tier"
          value={tier}
          onChange={(e) => setTier(e.target.value)}
        >
          <MenuItem value={ALL}>All tiers</MenuItem>
          {byTier.map((t) => (
            <MenuItem key={filterKey(t.service_tier)} value={filterKey(t.service_tier)}>
              {tierLabel(t.service_tier)}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="small" data-testid="service-tier-models">
          <TableHead>
            <TableRow>
              <TableCell>Model</TableCell>
              <TableCell>Tier</TableCell>
              <TableCell align="right">Requests</TableCell>
              <TableCell align="right">Prompt tokens</TableCell>
              <TableCell align="right">Response tokens</TableCell>
              <TableCell align="right">Cost</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {models.map((m) => (
              <TableRow key={`${m.service_tier}/${m.vendor}/${m.model}/${m.currency}`}>
                <TableCell>
                  {m.model}
                  <Typography component="span" variant="caption" color="text.secondary">
                    {` (${m.vendor})`}
                  </Typography>
                </TableCell>
                <TableCell>{tierLabel(m.service_tier)}</TableCell>
                <TableCell align="right">{m.requests.toLocaleString()}</TableCell>
                <TableCell align="right">{m.prompt_tokens.toLocaleString()}</TableCell>
                <TableCell align="right">{m.response_tokens.toLocaleString()}</TableCell>
                <TableCell align="right">{formatAmount(m.cost, m.currency)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    </Paper>
  );
};

export default ServiceTierUsagePanel;
