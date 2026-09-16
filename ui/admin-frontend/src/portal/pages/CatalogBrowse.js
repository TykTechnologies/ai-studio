import React, { useCallback, useEffect, useMemo, useState } from "react";
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
import { useDebounce } from "use-debounce";
import {
  TitleBox,
  ContentBox,
  StyledTextField,
} from "../../admin/styles/sharedStyles";
import SearchInput from "../../admin/components/common/SearchInput";
import EmptyStateWidget from "../../admin/components/common/EmptyStateWidget";
import PaginationControls from "../../admin/components/common/PaginationControls";
import AssetCard from "../components/catalog/AssetCard";
import usePortalCatalog from "../hooks/usePortalCatalog";
import {
  CATALOG_TYPES,
  DEFAULT_PAGE_SIZE,
  DEFAULT_SORT,
  SORT_OPTIONS,
  browsePath,
  itemKey,
  kindFacetLabel,
  privacyOptions,
  typeLabel,
} from "../utils/catalog";

/**
 * The portal's one searchable catalog (UX review D4). Everything the caller
 * can build with is one grid of cards, newest first, with search and filters
 * for type, kind (vendor / store / protocol), privacy level, catalog and
 * community submissions. Search, filters, sort and paging are applied by the
 * server (GET /common/catalog); the page holds one page of results and the
 * facets the server reports for the whole accessible set. The type may be
 * fixed by the route (/portal/catalog/llms) so the sidebar can link straight
 * to one type; every other control lives in the query string so a filtered
 * view can be shared and survives a refresh.
 */

const TYPE_TABS = [CATALOG_TYPES.LLM, CATALOG_TYPES.DATASOURCE, CATALOG_TYPES.TOOL];
const SEARCH_DEBOUNCE_MS = 350;

const CatalogBrowse = ({ type: routeType = "" }) => {
  const { pluginId, slug } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // A plugin resource route fixes the kind as well as the type.
  const routeKind = routeType === CATALOG_TYPES.PLUGIN_RESOURCE && pluginId && slug ? `${pluginId}:${slug}` : "";

  const q = searchParams.get("q") || "";
  const sort = searchParams.get("sort") || DEFAULT_SORT;
  const kind = routeKind || searchParams.get("kind") || "";
  const privacy = searchParams.get("privacy") || "";
  const catalog = searchParams.get("catalog") || "";
  const community = searchParams.get("community") === "1";
  const page = Math.max(1, parseInt(searchParams.get("page") || "1", 10) || 1);
  const pageSize = parseInt(searchParams.get("page_size") || "", 10) || DEFAULT_PAGE_SIZE;

  // Typing is debounced before it reaches the URL (and so the server); an
  // external change to q (back button, the overview's search box) syncs
  // the box without echoing.
  const [searchInput, setSearchInput] = useState(q);
  const [debouncedSearch] = useDebounce(searchInput, SEARCH_DEBOUNCE_MS);
  useEffect(() => {
    setSearchInput(q);
  }, [q]);

  const setParams = useCallback(
    (changes) => {
      const next = new URLSearchParams(searchParams);
      Object.entries(changes).forEach(([key, value]) => {
        if (value === "" || value === null || value === undefined || value === false) {
          next.delete(key);
        } else {
          next.set(key, value === true ? "1" : String(value));
        }
      });
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams],
  );

  useEffect(() => {
    if (debouncedSearch !== q) {
      // A new search starts from the first page.
      setParams({ q: debouncedSearch, page: "" });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearch]);

  const request = useMemo(
    () => ({
      q,
      type: routeType,
      kind,
      privacy,
      catalog,
      community,
      sort: sort === DEFAULT_SORT ? "" : sort,
      page: page > 1 ? page : "",
      page_size: pageSize !== DEFAULT_PAGE_SIZE ? pageSize : "",
    }),
    [q, routeType, kind, privacy, catalog, community, sort, page, pageSize],
  );
  const { items, meta, loading, error } = usePortalCatalog(request);

  // A filter change restarts paging.
  const setFilter = (key, value) => setParams({ [key]: value, page: "" });

  const clearFilters = () => {
    const next = new URLSearchParams();
    if (q) next.set("q", q);
    if (sort !== DEFAULT_SORT) next.set("sort", sort);
    setSearchParams(next, { replace: true });
  };

  // Switching type keeps the search term and sort, drops the type-specific
  // filters (kind, catalog) and the page.
  const goToType = (nextType) => {
    const next = new URLSearchParams();
    if (q) next.set("q", q);
    if (sort !== DEFAULT_SORT) next.set("sort", sort);
    if (privacy) next.set("privacy", privacy);
    if (community) next.set("community", "1");
    const query = next.toString();
    navigate(`${browsePath(nextType === "all" ? "" : nextType)}${query ? `?${query}` : ""}`);
  };

  const counts = meta?.counts || {};
  const accessibleTotal = Object.values(counts).reduce((sum, n) => sum + n, 0);
  const resourceTypes = meta?.resource_types || [];
  const kinds = useMemo(() => {
    const facets = meta?.kinds || [];
    return routeType ? facets.filter((facet) => facet.type === routeType) : facets;
  }, [meta, routeType]);
  const catalogs = useMemo(() => {
    const options = meta?.catalogs || [];
    return routeType ? options.filter((option) => option.type === routeType) : options;
  }, [meta, routeType]);
  const hasFilters = Boolean((!routeKind && kind) || privacy || catalog || community);

  const heading = routeKind
    ? resourceTypes.find((rt) => `${rt.plugin_id}:${rt.slug}` === routeKind)?.name || "Resources"
    : routeType
      ? typeLabel(routeType, { plural: true })
      : "Browse";

  const tabValue = routeKind || routeType || "all";

  const emptyState = () => {
    if (meta && accessibleTotal === 0) {
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
              setSearchInput("");
              setSearchParams(next, { replace: true });
            }}
          >
            Clear search and filters
          </Button>
        )}
      </Box>
    );
  };

  const total = meta?.total ?? 0;
  const firstOnPage = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const lastOnPage = Math.min(total, (page - 1) * pageSize + items.length);

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
            <Tab value="all" label={`All${meta ? ` (${accessibleTotal})` : ""}`} />
            {[
              ...TYPE_TABS,
              ...((counts[CATALOG_TYPES.MCP_SERVER] || 0) > 0 || routeType === CATALOG_TYPES.MCP_SERVER ? [CATALOG_TYPES.MCP_SERVER] : []),
            ].map((t) => (
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
            value={searchInput}
            onChange={setSearchInput}
            placeholder="Search by name, description, model, operation or vendor"
          />
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "center" }}>
            {!routeKind && kinds.length > 1 && (
              <StyledTextField
                select
                size="small"
                label={routeType === CATALOG_TYPES.LLM ? "Vendor" : routeType === CATALOG_TYPES.DATASOURCE ? "Store" : routeType === CATALOG_TYPES.TOOL ? "Protocol" : "Kind"}
                value={kind}
                onChange={(event) => setFilter("kind", event.target.value)}
                sx={{ minWidth: 180 }}
                inputProps={{ "data-testid": "filter-kind" }}
              >
                <MenuItem value="">Any</MenuItem>
                {kinds.map((facet) => (
                  <MenuItem key={`${facet.type}:${facet.kind}`} value={facet.kind}>
                    {kindFacetLabel(facet)}
                    {routeType ? "" : ` (${typeLabel(facet.type, { plural: true })})`}
                  </MenuItem>
                ))}
              </StyledTextField>
            )}
            <StyledTextField
              select
              size="small"
              label="Privacy level"
              value={privacy}
              onChange={(event) => setFilter("privacy", event.target.value)}
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
                onChange={(event) => setFilter("catalog", event.target.value)}
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
              onChange={(event) => setParams({ sort: event.target.value === DEFAULT_SORT ? "" : event.target.value, page: "" })}
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
                  onChange={(event) => setFilter("community", event.target.checked)}
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

        {loading && !meta ? (
          <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
            <CircularProgress />
          </Box>
        ) : error ? (
          <Typography color="error">The catalog could not be loaded. Please try again later.</Typography>
        ) : total === 0 ? (
          emptyState()
        ) : (
          <Box aria-busy={loading}>
            <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mb: 1.5 }} data-testid="catalog-count">
              {total <= pageSize
                ? `${total} ${total === 1 ? "asset" : "assets"}`
                : `${firstOnPage}–${lastOnPage} of ${total} assets`}
            </Typography>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))",
                gap: 2.5,
                opacity: loading ? 0.6 : 1,
                transition: "opacity 120ms ease",
              }}
              data-testid="catalog-grid"
            >
              {items.map((item) => (
                <AssetCard key={itemKey(item)} item={item} showType={!routeType} />
              ))}
            </Box>
            {(meta?.total_pages || 1) > 1 && (
              <PaginationControls
                page={page}
                pageSize={pageSize}
                totalPages={meta.total_pages}
                onPageChange={(event, value) => setParams({ page: value > 1 ? value : "" })}
                onPageSizeChange={(event) =>
                  setParams({ page_size: Number(event.target.value) !== DEFAULT_PAGE_SIZE ? event.target.value : "", page: "" })
                }
              />
            )}
          </Box>
        )}
      </ContentBox>
    </>
  );
};

export default CatalogBrowse;
