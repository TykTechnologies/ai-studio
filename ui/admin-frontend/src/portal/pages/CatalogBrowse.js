import React, { useCallback, useMemo } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  Box,
  CircularProgress,
  FormControlLabel,
  MenuItem,
  Switch,
  Tab,
  Tabs,
  Typography,
  Button,
} from "@mui/material";
import {
  TitleBox,
  ContentBox,
  StyledTextField,
} from "../../admin/styles/sharedStyles";
import SearchInput from "../../admin/components/common/SearchInput";
import EmptyStateWidget from "../../admin/components/common/EmptyStateWidget";
import AssetCard from "../components/catalog/AssetCard";
import usePortalCatalog from "../hooks/usePortalCatalog";
import {
  CATALOG_TYPES,
  DEFAULT_SORT,
  SORT_OPTIONS,
  browsePath,
  filterCatalogItems,
  itemKey,
  kindOptions,
  privacyOptions,
  sortCatalogItems,
  typeLabel,
} from "../utils/catalog";

/**
 * The portal's one searchable catalog (UX review D4). Everything the caller
 * can build with is one grid of cards, newest first, with search and filters
 * for type, kind (vendor / store / protocol), privacy level, catalog and
 * community submissions. The type may be fixed by the route
 * (/portal/catalog/llms) so the sidebar can link straight to one type;
 * every other filter lives in the query string so a filtered view can be
 * shared and survives a refresh.
 */

const TYPE_TABS = [CATALOG_TYPES.LLM, CATALOG_TYPES.DATASOURCE, CATALOG_TYPES.TOOL];

const CatalogBrowse = ({ type: routeType = "" }) => {
  const { pluginId, slug } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { items, meta, loading, error } = usePortalCatalog();

  // A plugin resource route fixes the kind as well as the type.
  const routeKind = routeType === CATALOG_TYPES.PLUGIN_RESOURCE && pluginId && slug ? `${pluginId}:${slug}` : "";

  const q = searchParams.get("q") || "";
  const sort = searchParams.get("sort") || DEFAULT_SORT;
  const kind = routeKind || searchParams.get("kind") || "";
  const privacy = searchParams.get("privacy") || "";
  const catalog = searchParams.get("catalog") || "";
  const community = searchParams.get("community") === "1";

  const setParam = useCallback(
    (key, value) => {
      const next = new URLSearchParams(searchParams);
      if (value === "" || value === null || value === undefined || value === false) {
        next.delete(key);
      } else {
        next.set(key, value === true ? "1" : String(value));
      }
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams],
  );

  const clearFilters = () => {
    const next = new URLSearchParams();
    if (q) next.set("q", q);
    if (sort !== DEFAULT_SORT) next.set("sort", sort);
    setSearchParams(next, { replace: true });
  };

  // Switching type keeps the search term and sort, drops the type-specific
  // filters (kind, catalog).
  const goToType = (nextType) => {
    const next = new URLSearchParams();
    if (q) next.set("q", q);
    if (sort !== DEFAULT_SORT) next.set("sort", sort);
    if (privacy) next.set("privacy", privacy);
    if (community) next.set("community", "1");
    const query = next.toString();
    navigate(`${browsePath(nextType === "all" ? "" : nextType)}${query ? `?${query}` : ""}`);
  };

  const scoped = useMemo(
    () => filterCatalogItems(items, { type: routeType, kind: routeKind }),
    [items, routeType, routeKind],
  );
  const filtered = useMemo(
    () => sortCatalogItems(filterCatalogItems(scoped, { q, kind, privacy, catalog, community }), sort),
    [scoped, q, kind, privacy, catalog, community, sort],
  );

  const kinds = useMemo(() => kindOptions(scoped), [scoped]);
  const catalogs = useMemo(() => {
    const options = meta?.catalogs || [];
    return routeType ? options.filter((option) => option.type === routeType) : options;
  }, [meta, routeType]);
  const resourceTypes = meta?.resource_types || [];
  const counts = meta?.counts || {};
  const hasFilters = Boolean((!routeKind && kind) || privacy || catalog || community);

  const heading = routeKind
    ? resourceTypes.find((rt) => `${rt.plugin_id}:${rt.slug}` === routeKind)?.name || "Resources"
    : routeType
      ? typeLabel(routeType, { plural: true })
      : "Browse";

  const tabValue = routeKind || routeType || "all";

  const emptyState = () => {
    if (items.length === 0) {
      return (
        <EmptyStateWidget
          title="Nothing to build with yet"
          description="None of your teams has a catalog with an active LLM provider, data source or tool in it. Ask an administrator to add you to a team or publish something to one of your catalogs."
        />
      );
    }
    return (
      <Box sx={{ textAlign: "center", py: 6 }} data-testid="catalog-no-matches">
        <Typography variant="headingMedium" component="p" sx={{ mb: 1 }}>
          {q ? `No matches for “${q}”` : "No matches"}
        </Typography>
        <Typography variant="bodyMediumDefault" color="text.defaultSubdued" sx={{ mb: 2 }}>
          Try another search term or clear the filters.
        </Typography>
        {(q || hasFilters) && (
          <Button
            variant="outlined"
            onClick={() => {
              const next = new URLSearchParams();
              if (sort !== DEFAULT_SORT) next.set("sort", sort);
              setSearchParams(next, { replace: true });
            }}
          >
            Clear search and filters
          </Button>
        )}
      </Box>
    );
  };

  return (
    <>
      <TitleBox top="64px">
        <Box>
          <Typography variant="headingXLarge" component="h1">
            {heading}
          </Typography>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            Everything your teams can build apps with. Newest first.
          </Typography>
        </Box>
      </TitleBox>
      <ContentBox>
        {!routeKind && (
          <Tabs
            value={tabValue}
            onChange={(event, value) => goToType(value)}
            variant="scrollable"
            scrollButtons="auto"
            aria-label="Asset type"
            sx={{ mb: 2, borderBottom: (theme) => `1px solid ${theme.palette.border.neutralDefault}` }}
          >
            <Tab value="all" label={`All${meta ? ` (${meta.total})` : ""}`} />
            {TYPE_TABS.map((t) => (
              <Tab key={t} value={t} label={`${typeLabel(t, { plural: true })}${meta ? ` (${counts[t] || 0})` : ""}`} />
            ))}
            {resourceTypes.map((rt) => (
              <Tab
                key={`${rt.plugin_id}:${rt.slug}`}
                value={`${rt.plugin_id}:${rt.slug}`}
                label={rt.name}
                onClick={() => navigate(browsePath(CATALOG_TYPES.PLUGIN_RESOURCE, { attributes: { resource_type: rt } }))}
              />
            ))}
          </Tabs>
        )}

        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 1.5,
            mb: 3,
          }}
        >
          <SearchInput
            value={q}
            onChange={(value) => setParam("q", value)}
            placeholder="Search by name, description, model, operation or vendor"
          />
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "center" }}>
            {!routeKind && kinds.length > 1 && (
              <StyledTextField
                select
                size="small"
                label={routeType === CATALOG_TYPES.LLM ? "Vendor" : routeType === CATALOG_TYPES.DATASOURCE ? "Store" : routeType === CATALOG_TYPES.TOOL ? "Protocol" : "Kind"}
                value={kind}
                onChange={(event) => setParam("kind", event.target.value)}
                sx={{ minWidth: 180 }}
                inputProps={{ "data-testid": "filter-kind" }}
              >
                <MenuItem value="">Any</MenuItem>
                {kinds.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </StyledTextField>
            )}
            <StyledTextField
              select
              size="small"
              label="Privacy level"
              value={privacy}
              onChange={(event) => setParam("privacy", event.target.value)}
              sx={{ minWidth: 180 }}
              inputProps={{ "data-testid": "filter-privacy" }}
            >
              <MenuItem value="">Any</MenuItem>
              {privacyOptions().map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </StyledTextField>
            {catalogs.length > 1 && (
              <StyledTextField
                select
                size="small"
                label="Catalog"
                value={catalog}
                onChange={(event) => setParam("catalog", event.target.value)}
                sx={{ minWidth: 200 }}
                inputProps={{ "data-testid": "filter-catalog" }}
              >
                <MenuItem value="">Any</MenuItem>
                {catalogs.map((option) => (
                  <MenuItem key={`${option.type}:${option.id}`} value={`${option.type}:${option.id}`}>
                    {routeType ? option.name : `${option.name} (${typeLabel(option.type, { plural: true })})`}
                  </MenuItem>
                ))}
              </StyledTextField>
            )}
            <StyledTextField
              select
              size="small"
              label="Sort"
              value={sort}
              onChange={(event) => setParam("sort", event.target.value === DEFAULT_SORT ? "" : event.target.value)}
              sx={{ minWidth: 200 }}
              inputProps={{ "data-testid": "sort" }}
            >
              {SORT_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </StyledTextField>
            <FormControlLabel
              control={
                <Switch
                  size="small"
                  checked={community}
                  onChange={(event) => setParam("community", event.target.checked)}
                />
              }
              label="Community submissions only"
            />
            {hasFilters && (
              <Button size="small" onClick={clearFilters} data-testid="clear-filters">
                Clear filters
              </Button>
            )}
          </Box>
        </Box>

        {loading ? (
          <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
            <CircularProgress />
          </Box>
        ) : error ? (
          <Typography color="error">The catalog could not be loaded. Please try again later.</Typography>
        ) : filtered.length === 0 ? (
          emptyState()
        ) : (
          <>
            <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mb: 1.5 }} data-testid="catalog-count">
              {filtered.length === scoped.length
                ? `${filtered.length} ${filtered.length === 1 ? "asset" : "assets"}`
                : `${filtered.length} of ${scoped.length} assets`}
            </Typography>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))",
                gap: 2.5,
              }}
              data-testid="catalog-grid"
            >
              {filtered.map((item) => (
                <AssetCard key={itemKey(item)} item={item} showType={!routeType} />
              ))}
            </Box>
          </>
        )}
      </ContentBox>
    </>
  );
};

export default CatalogBrowse;
