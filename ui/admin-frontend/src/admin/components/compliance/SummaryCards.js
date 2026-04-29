import React from "react";
import { Box, Paper, Typography } from "@mui/material";
import { styled } from "@mui/material/styles";
import TrendingUpIcon from "@mui/icons-material/TrendingUp";
import TrendingDownIcon from "@mui/icons-material/TrendingDown";
import TrendingFlatIcon from "@mui/icons-material/TrendingFlat";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import BlockIcon from "@mui/icons-material/Block";
import AccountBalanceWalletIcon from "@mui/icons-material/AccountBalanceWallet";
import ReportProblemIcon from "@mui/icons-material/ReportProblem";
import PolicyIcon from "@mui/icons-material/Policy";

const SummaryCard = styled(Paper)(({ theme, severity }) => {
  const colors = {
    error: {
      background: theme.palette.error.light + "20",
      border: theme.palette.error.main,
      icon: theme.palette.error.main,
    },
    warning: {
      background: theme.palette.warning.light + "20",
      border: theme.palette.warning.main,
      icon: theme.palette.warning.main,
    },
    info: {
      background: theme.palette.info.light + "20",
      border: theme.palette.info.main,
      icon: theme.palette.info.main,
    },
    success: {
      background: theme.palette.success.light + "20",
      border: theme.palette.success.main,
      icon: theme.palette.success.main,
    },
  };

  const color = colors[severity] || colors.info;

  return {
    padding: theme.spacing(2.5),
    borderRadius: theme.shape.borderRadius * 2,
    backgroundColor: color.background,
    borderLeft: `4px solid ${color.border}`,
    boxShadow: "none",
    height: "100%",
    display: "flex",
    flexDirection: "column",
    justifyContent: "space-between",
  };
});

const TrendIndicator = ({ value }) => {
  if (value > 0) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", color: "error.main" }}>
        <TrendingUpIcon fontSize="small" />
        <Typography variant="caption" sx={{ ml: 0.5 }}>
          +{value.toFixed(1)}%
        </Typography>
      </Box>
    );
  } else if (value < 0) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", color: "success.main" }}>
        <TrendingDownIcon fontSize="small" />
        <Typography variant="caption" sx={{ ml: 0.5 }}>
          {value.toFixed(1)}%
        </Typography>
      </Box>
    );
  }
  return (
    <Box sx={{ display: "flex", alignItems: "center", color: "text.secondary" }}>
      <TrendingFlatIcon fontSize="small" />
      <Typography variant="caption" sx={{ ml: 0.5 }}>
        No change
      </Typography>
    </Box>
  );
};

// Display thresholds for filter-script compliance event cards.
// "Warning" events are advisory (e.g. PII redaction that allowed the request
// through), so a small number is normal — only escalate the card colour once
// they accumulate. "Critical" events represent a serious filter-flagged
// concern; even a single one in the period warrants attention.
const COMPLIANCE_EVENTS_WARNING_ESCALATE_AT = 20;
const COMPLIANCE_EVENTS_CRITICAL_ESCALATE_AT = 0;

const getSeverity = (type, value) => {
  switch (type) {
    case "auth":
      return value > 20 ? "error" : value > 5 ? "warning" : "success";
    case "policy":
      return value > 50 ? "error" : value > 10 ? "warning" : "success";
    case "budget":
      return value > 3 ? "error" : value > 0 ? "warning" : "success";
    case "error":
      return value > 5 ? "error" : value > 1 ? "warning" : "success";
    case "events_critical":
      return value > COMPLIANCE_EVENTS_CRITICAL_ESCALATE_AT ? "error" : "success";
    case "events_warning":
      return value > COMPLIANCE_EVENTS_WARNING_ESCALATE_AT
        ? "warning"
        : value > 0
        ? "info"
        : "success";
    default:
      return "info";
  }
};

const SummaryCards = ({ summary }) => {
  if (!summary) return null;

  const cards = [
    {
      type: "auth",
      title: "Auth Failures",
      value: summary.auth_failures,
      trend: summary.auth_failures_trend,
      icon: ErrorOutlineIcon,
      description: "401 errors in period",
    },
    {
      type: "policy",
      title: "Policy Violations",
      value: summary.policy_violations,
      trend: summary.policy_trend,
      icon: BlockIcon,
      description: "403 blocked requests",
    },
    {
      type: "budget",
      title: "Budget Alerts",
      value: summary.budget_alerts,
      trend: summary.budget_trend,
      icon: AccountBalanceWalletIcon,
      description: "Apps over 80% budget",
    },
    {
      type: "error",
      title: "Error Rate",
      value: summary.error_rate?.toFixed(2) + "%",
      trend: summary.error_trend,
      icon: ReportProblemIcon,
      description: "5xx error percentage",
      rawValue: summary.error_rate,
    },
    {
      type: "events_critical",
      title: "Critical Events",
      value: summary.compliance_events_critical || 0,
      trend: summary.compliance_events_critical_trend,
      icon: PolicyIcon,
      description: "Critical filter-script events",
    },
    {
      type: "events_warning",
      title: "Warning Events",
      value: summary.compliance_events_warning || 0,
      trend: summary.compliance_events_warning_trend,
      icon: PolicyIcon,
      description: "Warning filter-script events",
    },
  ];

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: {
          xs: "1fr",
          sm: "repeat(2, 1fr)",
          md: "repeat(3, 1fr)",
        },
        gap: 2,
      }}
    >
      {cards.map((card) => {
        const Icon = card.icon;
        const severity = getSeverity(card.type, card.rawValue ?? card.value);

        return (
          <SummaryCard key={card.type} severity={severity}>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <Box>
                <Typography variant="subtitle2" color="text.secondary">
                  {card.title}
                </Typography>
                <Typography variant="h4" sx={{ my: 1, fontWeight: "bold" }}>
                  {card.value}
                </Typography>
              </Box>
              <Icon sx={{ fontSize: 40, opacity: 0.7 }} />
            </Box>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" color="text.secondary">
                {card.description}
              </Typography>
              <TrendIndicator value={card.trend} />
            </Box>
          </SummaryCard>
        );
      })}
    </Box>
  );
};

export default SummaryCards;
