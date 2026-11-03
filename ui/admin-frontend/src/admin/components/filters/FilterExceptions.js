import React, { useEffect, useRef, useState } from "react";
import { Alert, Table, TableBody, TableContainer, TableHead, TableRow, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import Section from "../common/Section";
import apiClient from "../../utils/apiClient";
import { StyledTableCell, StyledTableHeaderCell, StyledTableRow } from "../../styles/sharedStyles";
import PromptTemplateStatus from "../apps/promptTemplates/PromptTemplateStatus";
import {
  apiErrorMessage,
  channelLabel,
  exceptionActionLabel,
  listFilterGuardrailExceptions,
} from "../../services/guardrailExceptionService";

/** useAppNames resolves app ids to names, once each; "App #id" otherwise. */
const useAppNames = (ids) => {
  const [names, setNames] = useState({});
  const asked = useRef(new Set());
  const key = [...new Set(ids.map(String))].sort().join(",");
  useEffect(() => {
    key
      .split(",")
      .filter(Boolean)
      .forEach((id) => {
        if (asked.current.has(id)) return;
        asked.current.add(id);
        apiClient
          .get(`/apps/${id}`)
          .then((r) => {
            const n = r?.data?.data?.attributes?.name;
            if (n) setNames((prev) => ({ ...prev, [id]: n }));
          })
          .catch(() => {});
      });
  }, [key]);
  return (id) => names[String(id)] || `App #${id}`;
};

// FilterExceptions lists, read-only, the guardrail exceptions that soften this
// filter for some app, so the filter's owner sees what is softened and where.
// Exceptions are managed on each app's page.
const FilterExceptions = ({ filterId }) => {
  const [exceptions, setExceptions] = useState(null);
  const [error, setError] = useState(null);
  const appName = useAppNames((exceptions || []).map((e) => e.app_id));

  useEffect(() => {
    let cancelled = false;
    listFilterGuardrailExceptions(filterId)
      .then((list) => {
        if (!cancelled) setExceptions(list);
      })
      .catch((e) => {
        if (!cancelled) setError(apiErrorMessage(e, "Could not load the exceptions"));
      });
    return () => {
      cancelled = true;
    };
  }, [filterId]);

  const shown = (exceptions || []).filter((e) => e.status !== "retired");

  return (
    <Section
      title="Exceptions"
      description="Apps for which this filter does something other than its own action. Managed on each app's page."
      data-testid="filter-exceptions"
    >
      {error && <Alert severity="error">{error}</Alert>}
      {exceptions && shown.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          No app softens this filter.
        </Typography>
      )}
      {shown.length > 0 && (
        <TableContainer>
          <Table size="small" aria-label="Exceptions of this filter">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>App</StyledTableHeaderCell>
                <StyledTableHeaderCell>Detector</StyledTableHeaderCell>
                <StyledTableHeaderCell>Where</StyledTableHeaderCell>
                <StyledTableHeaderCell>Instead of blocking</StyledTableHeaderCell>
                <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                <StyledTableHeaderCell>Hits</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {shown.map((e) => (
                <StyledTableRow key={e.id} data-testid={`filter-exception-${e.id}`}>
                  <StyledTableCell>
                    <RouterLink to={`/admin/apps/${e.app_id}`}>{appName(e.app_id)}</RouterLink>
                  </StyledTableCell>
                  <StyledTableCell sx={{ fontFamily: "monospace" }}>{e.detector || "Any"}</StyledTableCell>
                  <StyledTableCell>
                    {channelLabel(e.channel)}
                    {e.tool_name ? ` (${e.tool_name})` : ""}
                  </StyledTableCell>
                  <StyledTableCell>{exceptionActionLabel(e.action)}</StyledTableCell>
                  <StyledTableCell>
                    <PromptTemplateStatus status={e.status} />
                  </StyledTableCell>
                  <StyledTableCell>{e.hit_count || 0}</StyledTableCell>
                </StyledTableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Section>
  );
};

export default FilterExceptions;
