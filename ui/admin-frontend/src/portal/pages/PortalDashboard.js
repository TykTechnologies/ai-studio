import React, { useEffect, useMemo, useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import {
  Box,
  CircularProgress,
  LinearProgress,
  Typography,
  styled,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import pubClient from "../../admin/utils/pubClient";
import useSystemFeatures from "../../admin/hooks/useSystemFeatures";
import useUserEntitlements from "../../admin/hooks/useUserEntitlements";
import Section from "../../admin/components/common/Section";
import DataTable from "../../admin/components/common/DataTable";
import EmptyStateWidget from "../../admin/components/common/EmptyStateWidget";
import SearchInput from "../../admin/components/common/SearchInput";
import IconBadge from "../../admin/components/common/IconBadge";
import { TitleBox, ContentBox, PrimaryButton, SecondaryLinkButton } from "../../admin/styles/sharedStyles";
import { relativeTime } from "../../admin/components/notifications/notificationPresentation";
import AppStatusChip, { getAppStatus } from "../components/AppStatusChip";
import AssetCard from "../components/catalog/AssetCard";
import usePortalCatalog from "../hooks/usePortalCatalog";
import {
  CATALOG_TYPES,
  browsePath,
  itemKey,
  typeIcon,
  typeLabel,
} from "../utils/catalog";

/**
 * The developer's landing page (UX review D4 / F-09). It answers the two
 * questions a developer arrives with: "what is the state of my apps?" (status,
 * budget, last access) and "what can I build with?" (a search box, the counts
 * per asset type and the newest assets), each linking on to the full page.
 */

const RECENT_LIMIT = 6;
const APPS_LIMIT = 6;

// The overview only needs the newest few assets; the counts and kinds it
// shows are the server's facets over everything the user can use.
const RECENT_REQUEST = { sort: "newest", page_size: RECENT_LIMIT };

const CountTile = styled(Box)(({ theme }) => ({
  display: "flex",
  alignItems: "center",
  gap: theme.spacing(1.5),
  padding: theme.spacing(1.5, 2),
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: "8px",
  backgroundColor: theme.palette.background.paper,
  textDecoration: "none",
  color: theme.palette.text.primary,
  minWidth: 200,
  flex: "1 1 200px",
  boxSizing: "border-box",
  "&:hover": {
    borderColor: theme.palette.border.neutralHovered,
    backgroundColor: theme.palette.background.surfaceNeutralHover,
  },
}));

// Budgets are dollar figures throughout the product (the app page prints
// them as "$x.xx"); the same plain form is used here.
const formatMoney = (value) => {
  if (typeof value !== "number" || Number.isNaN(value)) return "—";
  return `$${value.toFixed(2)}`;
};

/** Spend against budget, or what stands in for it. */
export const BudgetCell = ({ summary, spendTracked }) => {
  if (!summary) {
    return (
      <Typography variant="bodySmallDefault" color="text.defaultSubdued">
        —
      </Typography>
    );
  }
  const budget = summary.monthly_budget;
  const spent = summary.current_spend || 0;
  if (!spendTracked) {
    return (
      <Typography variant="bodySmallDefault" color="text.defaultSubdued" data-testid="budget-cell">
        {typeof budget === "number" ? `${formatMoney(budget)} / month` : "No limit"}
      </Typography>
    );
  }
  if (typeof budget !== "number" || budget <= 0) {
    return (
      <Typography variant="bodySmallDefault" data-testid="budget-cell">
        {formatMoney(spent)} spent · No limit
      </Typography>
    );
  }
  const pct = Math.min(100, Math.max(0, (spent / budget) * 100));
  const color = pct >= 90 ? "error" : pct >= 70 ? "warning" : "primary";
  return (
    <Box sx={{ minWidth: 160 }} data-testid="budget-cell">
      <Typography variant="bodySmallDefault">
        {formatMoney(spent)} of {formatMoney(budget)}
      </Typography>
      <LinearProgress
        variant="determinate"
        value={pct}
        color={color}
        aria-label={`${Math.round(pct)}% of monthly budget used`}
        sx={{ height: 6, borderRadius: 3, mt: 0.5 }}
      />
    </Box>
  );
};

const PortalDashboard = () => {
  const navigate = useNavigate();
  const { features, loading: featuresLoading } = useSystemFeatures();
  const { userName, uiOptions } = useUserEntitlements();
  const catalog = usePortalCatalog(RECENT_REQUEST);
  const [apps, setApps] = useState([]);
  const [usage, setUsage] = useState({ data: {}, spend_tracked: false });
  const [appsLoading, setAppsLoading] = useState(true);
  const [appsError, setAppsError] = useState(null);
  const [query, setQuery] = useState("");

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [appsResponse, usageResponse] = await Promise.all([
          pubClient.get("/common/apps", { params: { all: true } }),
          pubClient.get("/common/apps/usage-summary").catch(() => ({ data: { data: {}, spend_tracked: false } })),
        ]);
        if (cancelled) return;
        setApps(appsResponse.data?.data || []);
        setUsage({
          data: usageResponse.data?.data || {},
          spend_tracked: Boolean(usageResponse.data?.spend_tracked),
        });
      } catch (err) {
        console.error("Error fetching apps:", err);
        if (!cancelled) setAppsError("Your apps could not be loaded. Please try again later.");
      } finally {
        if (!cancelled) setAppsLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, []);

  // Apps needing attention or recently used first: awaiting approval, then
  // by last access, newest first.
  const orderedApps = useMemo(() => {
    const lastAccess = (app) => {
      const value = usage.data[String(app.id)]?.last_access_at;
      return value ? new Date(value).getTime() : 0;
    };
    return [...apps].sort((x, y) => lastAccess(y) - lastAccess(x) || Number(y.id) - Number(x.id));
  }, [apps, usage]);

  const recent = catalog.items;
  const counts = catalog.meta?.counts || {};
  const accessibleTotal = Object.values(counts).reduce((sum, n) => sum + n, 0);
  const resourceTypes = catalog.meta?.resource_types || [];
  const kindFacets = catalog.meta?.kinds || [];

  const submitSearch = (event) => {
    event.preventDefault();
    const term = query.trim();
    navigate(term ? `/portal/catalog?q=${encodeURIComponent(term)}` : "/portal/catalog");
  };

  if (featuresLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", mt: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  const showPortalFeatures = (features.feature_portal || features.feature_gateway) && (uiOptions?.show_portal ?? true);

  const appColumns = [
    {
      field: "name",
      headerName: "App",
      renderCell: (app) => (
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="bodyMediumMedium" component="div" noWrap>
            {app.attributes.name}
          </Typography>
          {app.attributes.description && (
            <Typography variant="bodySmallDefault" component="div" color="text.defaultSubdued" noWrap sx={{ maxWidth: 360 }}>
              {app.attributes.description}
            </Typography>
          )}
        </Box>
      ),
    },
    {
      field: "status",
      headerName: "Status",
      renderCell: (app) => (
        <AppStatusChip
          status={getAppStatus({
            isActive: app.attributes.is_active,
            credentialActive: app.attributes.credential_active ?? app.attributes.credential?.active,
          })}
        />
      ),
    },
    {
      field: "budget",
      headerName: "Budget this month",
      renderCell: (app) => <BudgetCell summary={usage.data[String(app.id)]} spendTracked={usage.spend_tracked} />,
    },
    {
      field: "last_access",
      headerName: "Last access",
      sx: { whiteSpace: "nowrap" },
      renderCell: (app) => {
        const summary = usage.data[String(app.id)];
        const requests = summary?.requests_30d || 0;
        return (
          <Box>
            <Typography variant="bodySmallDefault" component="div" data-testid="last-access" noWrap>
              {summary?.last_access_at ? relativeTime(summary.last_access_at) : "Never"}
            </Typography>
            <Typography variant="bodySmallDefault" component="div" color="text.defaultSubdued" noWrap>
              {requests === 1 ? "1 request" : `${requests} requests`} in 30 days
            </Typography>
          </Box>
        );
      },
    },
    {
      field: "resources",
      headerName: "Uses",
      sx: { whiteSpace: "nowrap" },
      renderCell: (app) => {
        const a = app.attributes;
        const parts = [];
        const count = (ids, singular, plural) => {
          const n = (ids || []).length;
          if (n > 0) parts.push(`${n} ${n === 1 ? singular : plural}`);
        };
        count(a.llm_ids, "LLM provider", "LLM providers");
        count(a.datasource_ids, "data source", "data sources");
        count(a.tool_ids, "tool", "tools");
        return (
          <Typography variant="bodySmallDefault" color="text.defaultSubdued">
            {parts.length > 0 ? parts.join(", ") : "Nothing yet"}
          </Typography>
        );
      },
    },
  ];

  return (
    <>
      <TitleBox top="64px">
        <Box>
          <Typography variant="headingXLarge" component="h1">
            Hi {userName || "there"}
          </Typography>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            Your apps, and what you can build with.
          </Typography>
        </Box>
        {showPortalFeatures && (
          <PrimaryButton startIcon={<AddIcon />} onClick={() => navigate("/portal/app/new")}>
            Create app
          </PrimaryButton>
        )}
      </TitleBox>
      <ContentBox>
        {showPortalFeatures && (
          <Section
            title="Your apps"
            description="An app holds the credential your code uses to call the gateway. An administrator approves each new credential."
            actions={
              apps.length > 0 ? (
                <SecondaryLinkButton component={RouterLink} to="/portal/apps" endIcon={<ArrowForwardIcon />}>
                  All apps ({apps.length})
                </SecondaryLinkButton>
              ) : null
            }
            data-testid="overview-apps"
          >
            {appsLoading ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
                <CircularProgress size={24} />
              </Box>
            ) : appsError ? (
              <Typography color="error">{appsError}</Typography>
            ) : apps.length === 0 ? (
              <EmptyStateWidget
                title="Create your first app"
                description="Pick the LLM providers, data sources and tools it needs and you get a key and secret to call them with. An administrator approves the credential before it works."
                buttonText="Create app"
                buttonIcon={<AddIcon />}
                onButtonClick={() => navigate("/portal/app/new")}
              />
            ) : (
              <DataTable
                columns={appColumns}
                data={orderedApps.slice(0, APPS_LIMIT)}
                onRowClick={(app) => navigate(`/portal/apps/${app.id}`)}
                ariaLabel="Your apps"
              />
            )}
          </Section>
        )}

        <Section
          title="Build with"
          description="LLM providers, data sources and tools your teams can use, newest first."
          actions={
            <SecondaryLinkButton component={RouterLink} to="/portal/catalog" endIcon={<ArrowForwardIcon />}>
              Browse all
            </SecondaryLinkButton>
          }
          data-testid="overview-build-with"
        >
          <Box component="form" onSubmit={submitSearch} role="search" sx={{ mb: 2.5 }}>
            <SearchInput
              value={query}
              onChange={setQuery}
              placeholder="Search LLM providers, data sources and tools"
            />
          </Box>

          {catalog.loading ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
              <CircularProgress size={24} />
            </Box>
          ) : catalog.error ? (
            <Typography color="error">The catalog could not be loaded. Please try again later.</Typography>
          ) : accessibleTotal === 0 ? (
            <Typography variant="bodyMediumDefault" color="text.defaultSubdued" data-testid="overview-empty-catalog">
              None of your teams has a catalog with anything active in it yet. Ask an administrator to add you to a team or publish something to one of your catalogs.
            </Typography>
          ) : (
            <>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1.5, mb: 3 }} data-testid="overview-counts">
                {/* MCP servers and model routers are asset types like the
                    others; their tiles only show where the catalog has some. */}
                {[CATALOG_TYPES.LLM, CATALOG_TYPES.MODEL_ROUTER, CATALOG_TYPES.DATASOURCE, CATALOG_TYPES.TOOL, CATALOG_TYPES.MCP_SERVER]
                  .filter((type) => (counts[type] || 0) > 0)
                  .map((type) => (
                    <CountTile key={type} component={RouterLink} to={browsePath(type)}>
                      <IconBadge iconName={typeIcon(type)} />
                      <Box>
                        <Typography variant="headingLarge" component="div" sx={{ lineHeight: 1.1 }}>
                          {counts[type]}
                        </Typography>
                        <Typography variant="bodySmallDefault" color="text.defaultSubdued">
                          {typeLabel(type, { plural: counts[type] !== 1 })}
                        </Typography>
                      </Box>
                    </CountTile>
                  ))}
                {resourceTypes.map((rt) => {
                  const total =
                    kindFacets.find(
                      (facet) => facet.type === CATALOG_TYPES.PLUGIN_RESOURCE && facet.kind === `${rt.plugin_id}:${rt.slug}`,
                    )?.count || 0;
                  if (total === 0) return null;
                  return (
                    <CountTile
                      key={`${rt.plugin_id}:${rt.slug}`}
                      component={RouterLink}
                      to={browsePath(CATALOG_TYPES.PLUGIN_RESOURCE, { attributes: { resource_type: rt } })}
                    >
                      <IconBadge iconName="puzzle-piece" />
                      <Box>
                        <Typography variant="headingLarge" component="div" sx={{ lineHeight: 1.1 }}>
                          {total}
                        </Typography>
                        <Typography variant="bodySmallDefault" color="text.defaultSubdued">
                          {rt.name}
                        </Typography>
                      </Box>
                    </CountTile>
                  );
                })}
              </Box>

              <Typography variant="headingSmall" component="h3" sx={{ mb: 1.5 }}>
                Recently added
              </Typography>
              <Box
                sx={{
                  display: "grid",
                  gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
                  gap: 2,
                }}
                data-testid="overview-recent"
              >
                {recent.map((item) => (
                  <AssetCard key={itemKey(item)} item={item} compact />
                ))}
              </Box>
            </>
          )}
        </Section>
      </ContentBox>
    </>
  );
};

export default PortalDashboard;
