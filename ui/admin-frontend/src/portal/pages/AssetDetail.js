import React, { useEffect, useMemo, useState } from "react";
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Grid,
  IconButton,
  Link,
  Table,
  TableBody,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import DescriptionIcon from "@mui/icons-material/Description";
import { format } from "date-fns";
import pubClient from "../../admin/utils/pubClient";
import Section from "../../admin/components/common/Section";
import PrivacyLevelChip from "../../admin/components/common/privacy/PrivacyLevelChip";
import CommunityBadge from "../../admin/components/submissions/CommunityBadge";
import GovernedMetadataBadges from "../components/GovernedMetadataBadges";
import AssetAvatar from "../components/catalog/AssetAvatar";
import AssetTypeChip from "../components/catalog/AssetTypeChip";
import AppStatusChip, { getAppStatus } from "../components/AppStatusChip";
import {
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
  SecondaryLinkButton,
  SecondaryOutlineButton,
  StyledTableCell,
  StyledTableHeaderCell,
} from "../../admin/styles/sharedStyles";
import {
  CATALOG_TYPES,
  browsePath,
  buildActionLabel,
  buildAppPath,
  embedderLabel,
  isAppGranted,
  secondaryActionLabel,
  secondaryActionPath,
  formatPerMillion,
  itemKey,
  kindLabel,
  kindLogo,
  openAICompatibleBaseUrl,
  typeLabelLower,
} from "../utils/catalog";
import { generateSlug } from "../../admin/components/wizards/quick-start/utils";

/**
 * The detail page for one catalog asset (UX review D3: header with state
 * and the primary action, what it is, how to use it, who uses it). It
 * replaces the "More" modals, which showed a heading and a vendor, so a
 * developer can decide whether to build on an asset -- which models it
 * serves, at what privacy level, through which URL -- without creating an
 * app first.
 */

const DETAIL_PATHS = {
  [CATALOG_TYPES.LLM]: (params) => `/common/catalog/llms/${params.id}`,
  [CATALOG_TYPES.DATASOURCE]: (params) => `/common/catalog/datasources/${params.id}`,
  [CATALOG_TYPES.TOOL]: (params) => `/common/catalog/tools/${params.id}`,
  [CATALOG_TYPES.PLUGIN_RESOURCE]: (params) =>
    `/common/catalog/resources/${params.pluginId}/${params.slug}/${encodeURIComponent(params.instanceId)}`,
};

const APP_ID_FIELDS = {
  [CATALOG_TYPES.LLM]: "llm_ids",
  [CATALOG_TYPES.DATASOURCE]: "datasource_ids",
  [CATALOG_TYPES.TOOL]: "tool_ids",
};

const formatDate = (value) => {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : format(date, "d MMM yyyy");
};

const Field = ({ label, children, xs = 12, sm = 6, md = 3 }) => (
  <Grid item xs={xs} sm={sm} md={md}>
    <FieldLabel variant="bodySmallDefault">{label}</FieldLabel>
    <FieldValue variant="bodyMediumDefault" component="div">
      {children}
    </FieldValue>
  </Grid>
);

const CopyableCode = ({ value, label }) => {
  const copy = () => {
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(value).catch(() => undefined);
    }
  };
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
      <Typography
        component="code"
        variant="body2"
        sx={{
          fontFamily: "monospace",
          bgcolor: "action.hover",
          px: 1.5,
          py: 1,
          borderRadius: 1,
          flexGrow: 1,
          wordBreak: "break-all",
        }}
      >
        {value}
      </Typography>
      <Tooltip title={`Copy ${label}`}>
        <IconButton aria-label={`Copy ${label}`} size="small" onClick={copy}>
          <ContentCopyIcon fontSize="small" />
        </IconButton>
      </Tooltip>
    </Box>
  );
};

const LLMSections = ({ item }) => {
  const attrs = item.attributes || {};
  const models = attrs.models || [];
  const allowed = attrs.allowed_models || [];
  const baseUrl = openAICompatibleBaseUrl(attrs.name || "");
  const slug = generateSlug(attrs.name || "");
  const hasPrices = models.some((m) => typeof m.input_price_per_million === "number");
  const metadataEntries = Object.entries(attrs.metadata || {});

  return (
    <>
      <Section
        title="Models"
        description="What this provider serves through the gateway, as far as the platform knows."
        data-testid="llm-models-section"
      >
        {allowed.length > 0 ? (
          <Alert severity="info" sx={{ mb: 2 }}>
            <Typography variant="bodyMediumDefault" component="div">
              Only models matching this allow list are accepted. A request for any other model is rejected.
            </Typography>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 1 }}>
              {allowed.map((pattern) => (
                <Chip key={pattern} label={pattern} size="small" sx={{ fontFamily: "monospace" }} />
              ))}
            </Box>
          </Alert>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued" sx={{ mb: 2 }}>
            No allow list is set: any model the vendor serves can be requested. The default model is used when a request names none.
          </Typography>
        )}
        {models.length > 0 ? (
          <Table size="small" aria-label="Models">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>Model</StyledTableHeaderCell>
                <StyledTableHeaderCell>Role</StyledTableHeaderCell>
                {hasPrices && <StyledTableHeaderCell align="right">Input / 1M tokens</StyledTableHeaderCell>}
                {hasPrices && <StyledTableHeaderCell align="right">Output / 1M tokens</StyledTableHeaderCell>}
              </TableRow>
            </TableHead>
            <TableBody>
              {models.map((model) => (
                <TableRow key={model.name} data-testid="llm-model-row">
                  <StyledTableCell sx={{ fontFamily: "monospace" }}>{model.name}</StyledTableCell>
                  <StyledTableCell>
                    {model.is_default ? <Chip size="small" label="Default" color="primary" variant="outlined" /> : ""}
                  </StyledTableCell>
                  {hasPrices && (
                    <StyledTableCell align="right">
                      {typeof model.input_price_per_million === "number" ? formatPerMillion(model.input_price_per_million, model.currency) : "—"}
                    </StyledTableCell>
                  )}
                  {hasPrices && (
                    <StyledTableCell align="right">
                      {typeof model.output_price_per_million === "number" ? formatPerMillion(model.output_price_per_million, model.currency) : "—"}
                    </StyledTableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            No model names are recorded for this provider. Send the vendor's model name in the request's <code>model</code> field.
          </Typography>
        )}
      </Section>

      <Section title="How to call it" description="Endpoints your app credential can use once an administrator has approved it.">
        <FieldLabel variant="bodySmallDefault">OpenAI-compatible base URL</FieldLabel>
        <CopyableCode value={baseUrl} label="base URL" />
        <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mt: 1, mb: 2 }}>
          Use it as the base URL of any OpenAI client library with your app's key and secret. The app page lists every endpoint, including the unified ingress where this provider's models are addressed as{" "}
          <code>{slug}/{attrs.default_model || "<model>"}</code>.
        </Typography>
      </Section>

      {metadataEntries.length > 0 && (
        <Section title="Metadata" description="Additional facts recorded on this provider.">
          <Table size="small" aria-label="Metadata">
            <TableBody>
              {metadataEntries.map(([key, value]) => (
                <TableRow key={key}>
                  <StyledTableCell sx={{ fontFamily: "monospace", width: "30%" }}>{key}</StyledTableCell>
                  <StyledTableCell>{typeof value === "object" ? JSON.stringify(value) : String(value)}</StyledTableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Section>
      )}
    </>
  );
};

const DatasourceSections = ({ item }) => {
  const attrs = item.attributes || {};
  const logo = kindLogo(item);
  return (
    <Section title="Connection" description="How the data source is stored and embedded.">
      <Grid container spacing={2}>
        <Field label="Store">
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            {logo && <img src={logo} alt="" style={{ width: 20, height: 20, objectFit: "contain" }} />}
            {kindLabel(item) || "—"}
          </Box>
        </Field>
        <Field label="Embedding vendor">{embedderLabel(attrs.embed_vendor) || "—"}</Field>
        <Field label="Embedding model">{attrs.embed_model || "—"}</Field>
      </Grid>
      <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mt: 2 }}>
        Add this data source to an app to query it; the app page shows the search, vector search and embedding endpoints with examples.
      </Typography>
    </Section>
  );
};

const ToolSections = ({ item }) => {
  const attrs = item.attributes || {};
  const operations = attrs.operations || [];
  return (
    <Section
      title="Operations"
      description="What the tool exposes to your apps."
      actions={
        <SecondaryOutlineButton
          size="small"
          component={RouterLink}
          to={`/portal/tools/${item.id}/docs`}
          startIcon={<DescriptionIcon />}
        >
          API documentation
        </SecondaryOutlineButton>
      }
    >
      {operations.length > 0 ? (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
          {operations.map((operation) => (
            <Chip key={operation} label={operation} size="small" variant="outlined" sx={{ fontFamily: "monospace" }} />
          ))}
        </Box>
      ) : (
        <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
          No operations are listed for this tool.
        </Typography>
      )}
    </Section>
  );
};

const PluginResourceSections = ({ item }) => {
  const rt = item.attributes?.resource_type;
  if (!rt) return null;
  const granted = isAppGranted(item);
  const external = secondaryActionPath(item);
  return (
    <Section title={rt.name} description="Provided by a plugin.">
      <Typography variant="bodyMediumDefault" color="text.defaultSubdued" data-testid="plugin-resource-access-note">
        {granted
          ? `${rt.name} resources are attached to apps like any other asset; an app credential is what grants access to them.`
          : `Access to ${rt.name} resources is managed by the plugin that provides them, not through an app.`}
        {!granted && external && (
          <>
            {" "}
            <RouterLink to={external}>Open it there</RouterLink> to see the details and request access.
          </>
        )}
      </Typography>
    </Section>
  );
};

const AssetDetail = ({ type }) => {
  const params = useParams();
  const navigate = useNavigate();
  const [item, setItem] = useState(null);
  const [apps, setApps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const requestPath = DETAIL_PATHS[type]?.(params);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setItem(null);
    const load = async () => {
      try {
        const [detail, appList] = await Promise.all([
          pubClient.get(requestPath),
          pubClient.get("/common/apps", { params: { all: true } }).catch(() => ({ data: { data: [] } })),
        ]);
        if (cancelled) return;
        setItem(detail.data?.data || null);
        setApps(appList.data?.data || []);
      } catch (err) {
        if (cancelled) return;
        setError(err?.response?.status === 404 ? "not_found" : "failed");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [requestPath]);

  const usingApps = useMemo(() => {
    const field = APP_ID_FIELDS[type];
    if (!field || !item) return [];
    return apps.filter((app) => (app.attributes?.[field] || []).map(String).includes(String(item.id)));
  }, [apps, item, type]);

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", mt: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error || !item) {
    return (
      <ContentBox>
        <Typography variant="headingLarge" component="h1" gutterBottom>
          {error === "not_found" ? `This ${typeLabelLower(type)} is not available to you` : "Something went wrong"}
        </Typography>
        <Typography variant="bodyMediumDefault" color="text.defaultSubdued" sx={{ mb: 2 }}>
          {error === "not_found"
            ? "It may have been removed from your teams' catalogs, switched off, or never existed."
            : "The asset could not be loaded. Please try again later."}
        </Typography>
        <SecondaryLinkButton startIcon={<ArrowBackIcon />} component={RouterLink} to={browsePath(type)}>
          Back to browse
        </SecondaryLinkButton>
      </ContentBox>
    );
  }

  const attrs = item.attributes || {};
  const kind = kindLabel(item);
  const logo = kindLogo(item);
  const catalogs = attrs.catalogs || [];
  const tags = attrs.tags || [];
  const governed = Array.isArray(item.governed_metadata) ? item.governed_metadata : [];

  return (
    <>
      <TitleBox top="64px" sx={{ alignItems: "flex-start", gap: 2, flexWrap: "wrap" }}>
        <Box sx={{ display: "flex", gap: 2, alignItems: "center", minWidth: 0 }}>
          <AssetAvatar name={attrs.name} seed={itemKey(item)} logoUrl={attrs.logo_url} size={64} />
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="headingXLarge" component="h1" sx={{ wordBreak: "break-word" }}>
              {attrs.name}
            </Typography>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mt: 0.5 }}>
              <AssetTypeChip item={item} />
              {kind && (
                <Box sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}>
                  {logo && <img src={logo} alt="" style={{ width: 16, height: 16, objectFit: "contain" }} />}
                  <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
                    {kind}
                  </Typography>
                </Box>
              )}
              {attrs.privacy_score !== null && attrs.privacy_score !== undefined && (
                <PrivacyLevelChip score={attrs.privacy_score} />
              )}
              <CommunityBadge show={Boolean(attrs.community_submitted)} />
            </Box>
          </Box>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexShrink: 0 }}>
          <SecondaryLinkButton startIcon={<ArrowBackIcon />} component={RouterLink} to={browsePath(type, item)}>
            Back to browse
          </SecondaryLinkButton>
          {isAppGranted(item) ? (
            <PrimaryButton onClick={() => navigate(buildAppPath(item))} data-testid="asset-build-app">
              {buildActionLabel(item)}
            </PrimaryButton>
          ) : (
            secondaryActionPath(item) && (
              <SecondaryOutlineButton
                onClick={() => navigate(secondaryActionPath(item))}
                data-testid="asset-view-external"
              >
                {secondaryActionLabel(item)}
              </SecondaryOutlineButton>
            )
          )}
        </Box>
      </TitleBox>

      <ContentBox>
        <Section title="About">
          {attrs.short_description && (
            <Typography variant="bodyLargeMedium" component="p" sx={{ m: 0, mb: 1 }}>
              {attrs.short_description}
            </Typography>
          )}
          {attrs.long_description && (
            <Typography variant="bodyMediumDefault" component="p" color="text.defaultSubdued" sx={{ m: 0, mb: 2, whiteSpace: "pre-wrap" }}>
              {attrs.long_description}
            </Typography>
          )}
          {!attrs.short_description && !attrs.long_description && (
            <Typography variant="bodyMediumDefault" component="p" color="text.defaultSubdued" sx={{ m: 0, mb: 2 }}>
              No description has been provided.
            </Typography>
          )}
          <Grid container spacing={2}>
            <Field label="Added">{formatDate(attrs.created_at)}</Field>
            <Field label="Last updated">{formatDate(attrs.updated_at)}</Field>
            <Field label="Available through" md={6}>
              {catalogs.length > 0 ? (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }} data-testid="asset-catalogs">
                  {catalogs.map((catalog) => (
                    <Chip key={catalog.id} size="small" variant="outlined" label={catalog.name} />
                  ))}
                </Box>
              ) : (
                <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
                  {type === CATALOG_TYPES.PLUGIN_RESOURCE ? "Your team's resource grants" : "—"}
                </Typography>
              )}
            </Field>
            {tags.length > 0 && (
              <Field label="Tags" md={12}>
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                  {tags.map((tag) => (
                    <Chip key={tag} size="small" label={tag} />
                  ))}
                </Box>
              </Field>
            )}
          </Grid>
        </Section>

        {type === CATALOG_TYPES.LLM && <LLMSections item={item} />}
        {type === CATALOG_TYPES.DATASOURCE && <DatasourceSections item={item} />}
        {type === CATALOG_TYPES.TOOL && <ToolSections item={item} />}
        {type === CATALOG_TYPES.PLUGIN_RESOURCE && <PluginResourceSections item={item} />}

        {governed.length > 0 && (
          <Section title="Governance" description="Metadata the organisation records about this asset.">
            <GovernedMetadataBadges items={governed} />
          </Section>
        )}

        {APP_ID_FIELDS[type] && (
          <Section
            title="Your apps"
            description={`Apps of yours that already have access to this ${typeLabelLower(type)}.`}
          >
            {usingApps.length > 0 ? (
              <Box component="ul" sx={{ listStyle: "none", p: 0, m: 0, display: "flex", flexDirection: "column", gap: 1 }} data-testid="asset-apps">
                {usingApps.map((app) => (
                  <Box component="li" key={app.id} sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                    <Link component={RouterLink} to={`/portal/apps/${app.id}`} variant="bodyMediumMedium">
                      {app.attributes.name}
                    </Link>
                    <AppStatusChip
                      status={getAppStatus({
                        isActive: app.attributes.is_active,
                        credentialActive: app.attributes.credential_active ?? app.attributes.credential?.active,
                      })}
                    />
                  </Box>
                ))}
              </Box>
            ) : (
              <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
                None of your apps use this yet. {buildActionLabel(item) === "Get access" ? "Get access" : "Build an app"} to get a credential for it.
              </Typography>
            )}
          </Section>
        )}
      </ContentBox>
    </>
  );
};

export default AssetDetail;
