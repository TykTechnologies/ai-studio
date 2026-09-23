import React, { useEffect, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import {
  Alert,
  Box,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import { useEdition } from "../../context/EditionContext";
import { teamBudgetsService, errorDetail, formatMoney } from "../../services/teamBudgetsService";

// TeamCostsTable answers "how much is each team costing me?" over the
// dashboard's date range: proxy, edge and chat spend attributed to teams
// (Enterprise).
const TeamCostsTable = ({ startDate, endDate }) => {
  const { isEnterprise } = useEdition();
  const [costs, setCosts] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!isEnterprise) return;
    teamBudgetsService
      .getTeamCosts(startDate, endDate)
      .then((data) => {
        setCosts(data);
        setError("");
      })
      .catch((err) => setError(errorDetail(err, "Failed to load team costs")));
  }, [isEnterprise, startDate, endDate]);

  if (!isEnterprise) return null;

  const rows = [...(costs?.teams || [])].sort((a, b) => b.cost - a.cost);
  const unattributed = costs?.unattributed;
  const total = rows.reduce((sum, r) => sum + r.cost, 0) + (unattributed?.cost || 0);

  return (
    <Paper elevation={3} sx={{ p: 2, mt: 3 }} data-testid="team-costs">
      <Typography variant="h6" gutterBottom>
        Team Costs
      </Typography>
      {error && <Alert severity="error">{error}</Alert>}
      {costs && (
        <Box sx={{ overflowX: "auto" }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Team</TableCell>
                <TableCell align="right">Cost</TableCell>
                <TableCell align="right">Share</TableCell>
                <TableCell align="right">Tokens</TableCell>
                <TableCell align="right">Requests</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.team_id}>
                  <TableCell>
                    {row.deleted ? (
                      // A deleted team has no page, and a new team may carry
                      // its name.
                      <span data-testid={`team-costs-deleted-${row.team_id}`}>
                        {row.team_name}{" "}
                        <Typography component="span" variant="body2" color="text.secondary">
                          (deleted)
                        </Typography>
                      </span>
                    ) : (
                      <Link component={RouterLink} to={`/admin/groups/${row.team_id}`}>
                        {row.team_name}
                      </Link>
                    )}
                  </TableCell>
                  <TableCell align="right">{formatMoney(row.cost)}</TableCell>
                  <TableCell align="right">{total > 0 ? `${((row.cost / total) * 100).toFixed(1)}%` : "—"}</TableCell>
                  <TableCell align="right">{row.tokens.toLocaleString()}</TableCell>
                  <TableCell align="right">{row.requests.toLocaleString()}</TableCell>
                </TableRow>
              ))}
              {unattributed && unattributed.requests > 0 && (
                <TableRow data-testid="team-costs-unattributed">
                  <TableCell>
                    <em>No team</em>
                  </TableCell>
                  <TableCell align="right">{formatMoney(unattributed.cost)}</TableCell>
                  <TableCell align="right">{total > 0 ? `${((unattributed.cost / total) * 100).toFixed(1)}%` : "—"}</TableCell>
                  <TableCell align="right">{unattributed.tokens.toLocaleString()}</TableCell>
                  <TableCell align="right">{unattributed.requests.toLocaleString()}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </Box>
      )}
    </Paper>
  );
};

export default TeamCostsTable;
