import React, { useEffect, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import { Box, CircularProgress, Grid, Link, Typography } from "@mui/material";
import Section from "./Section";
import apiClient from "../../utils/apiClient";
import { FieldLabel } from "../../styles/sharedStyles";
import { fetchDependents, groupsForDependents } from "../../utils/dependentsMessage";

/**
 * "Used by" on an object's detail page (UX review M5 / F-07): one row per
 * non-empty dependent group (Apps, Catalogs, LLM providers, ...), each item a
 * link to its admin page. Reads `/api/v1/<resourcePath>/<id>/dependents`,
 * the same endpoint the delete confirmation uses, so the two never disagree.
 *
 * `extra` renders under the rows for callers that have something more to
 * say (a catalogue's teams, for example).
 */
const UsedBySection = ({ resourcePath, id, objectLabel = "object", extra = null, sx }) => {
  const [state, setState] = useState({ loading: true, dependents: null, failed: false });

  useEffect(() => {
    let cancelled = false;
    setState({ loading: true, dependents: null, failed: false });
    if (!id) return undefined;
    fetchDependents(resourcePath, id).then((dependents) => {
      if (cancelled) return;
      setState({ loading: false, dependents, failed: dependents === null });
    });
    return () => {
      cancelled = true;
    };
  }, [resourcePath, id]);

  const groups = groupsForDependents(state.dependents, { resourcePath });
  const total =
    typeof state.dependents?.total === "number"
      ? state.dependents.total
      : groups.reduce((sum, g) => sum + g.count, 0);

  let body;
  if (state.loading) {
    body = (
      <Box display="flex" alignItems="center" gap={1} data-testid="used-by-loading">
        <CircularProgress size={18} />
        <Typography variant="body2" color="text.secondary">
          Loading usage…
        </Typography>
      </Box>
    );
  } else if (state.failed) {
    body = (
      <Typography variant="body2" color="text.secondary" data-testid="used-by-error">
        Could not load usage.
      </Typography>
    );
  } else if (total === 0 || groups.length === 0) {
    body = (
      <Typography variant="body2" color="text.secondary" data-testid="used-by-empty">
        Nothing uses this {objectLabel} yet.
      </Typography>
    );
  } else {
    body = (
      <Grid container spacing={2} data-testid="used-by-groups">
        {groups.map((group) => (
          <React.Fragment key={group.key}>
            <Grid item xs={12} sm={3}>
              <FieldLabel>
                {group.label} ({group.count}):
              </FieldLabel>
            </Grid>
            <Grid item xs={12} sm={9}>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: "4px 16px" }} data-testid={`used-by-${group.key}`}>
                {group.items.map((item) => (
                  <Link key={item.id} component={RouterLink} to={item.href} underline="hover">
                    {item.name || `#${item.id}`}
                  </Link>
                ))}
              </Box>
            </Grid>
          </React.Fragment>
        ))}
      </Grid>
    );
  }

  return (
    <Section title="Used by" data-testid="used-by-section" sx={sx}>
      {body}
      {extra && <Box sx={{ mt: groups.length > 0 ? 2 : 1 }}>{extra}</Box>}
    </Section>
  );
};

export default UsedBySection;

/**
 * "Used by" for a catalogue: the teams that can see it, from
 * `/api/v1/<resourcePath>/<id>/groups` (name + member count, linking to the
 * team page). `resourcePath` is "catalogues", "data-catalogues" or
 * "tool-catalogues".
 */
export const CatalogueTeamsSection = ({ resourcePath, id }) => {
  const [state, setState] = useState({ loading: true, teams: [], failed: false });

  useEffect(() => {
    let cancelled = false;
    setState({ loading: true, teams: [], failed: false });
    if (!id) return undefined;
    apiClient
      .get(`/${resourcePath}/${id}/groups`)
      .then((response) => {
        if (cancelled) return;
        // Anything but the documented array (an SPA fallback page, a
        // JSON:API error body) is a failure, not "no teams".
        const teams = response.data?.data;
        if (!Array.isArray(teams)) {
          setState({ loading: false, teams: [], failed: true });
          return;
        }
        setState({ loading: false, teams, failed: false });
      })
      .catch((error) => {
        console.error(`Error fetching teams for ${resourcePath}/${id}`, error);
        if (!cancelled) setState({ loading: false, teams: [], failed: true });
      });
    return () => {
      cancelled = true;
    };
  }, [resourcePath, id]);

  let body;
  if (state.loading) {
    body = (
      <Box display="flex" alignItems="center" gap={1} data-testid="used-by-loading">
        <CircularProgress size={18} />
        <Typography variant="body2" color="text.secondary">
          Loading teams…
        </Typography>
      </Box>
    );
  } else if (state.failed) {
    body = (
      <Typography variant="body2" color="text.secondary" data-testid="used-by-error">
        Could not load usage.
      </Typography>
    );
  } else if (state.teams.length === 0) {
    body = (
      <Typography variant="body2" color="text.secondary" data-testid="used-by-empty">
        No teams use this catalog yet.
      </Typography>
    );
  } else {
    body = (
      <Grid container spacing={2} data-testid="used-by-groups">
        <Grid item xs={12} sm={3}>
          <FieldLabel>Teams ({state.teams.length}):</FieldLabel>
        </Grid>
        <Grid item xs={12} sm={9}>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: "4px 16px" }} data-testid="used-by-teams">
            {state.teams.map((team) => (
              <Typography key={team.id} variant="body2" component="span">
                <Link component={RouterLink} to={`/admin/groups/${team.id}`} underline="hover">
                  {team.name || `#${team.id}`}
                </Link>{" "}
                <Typography component="span" variant="body2" color="text.secondary">
                  ({team.member_count ?? 0} {team.member_count === 1 ? "member" : "members"})
                </Typography>
              </Typography>
            ))}
          </Box>
        </Grid>
      </Grid>
    );
  }

  return (
    <Section title="Used by" data-testid="used-by-section">
      {body}
    </Section>
  );
};
