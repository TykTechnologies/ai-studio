import React, { useEffect, useMemo, useState } from "react";
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Grid,
  Link,
  Table,
  TableBody,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
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
import CopyableCode, { CopyableBlock } from "../components/connect/CopyableCode";
import { toolMcpEnabled, toolRestEnabled } from "../utils/toolEndpoints";
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
  MCP_AUTH_LABELS,
  embedderLabel,
  isAppGranted,
  secondaryActionLabel,
  secondaryActionPath,
  formatPerMillion,
  itemKey,
  kindLabel,
  kindLogo,
  modelRouterDetailApiPath,
  openAICompatibleBaseUrl,
  typeLabelLower,
  unifiedIngressBaseUrl,
} from "../utils/catalog";
import { getVendorName } from "../../admin/utils/vendorLogos";
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
  [CATALOG_TYPES.MCP_SERVER]: (params) => `/common/catalog/mcp-servers/${params.id}`,
  [CATALOG_TYPES.MODEL_ROUTER]: (params) => modelRouterDetailApiPath(params.id),
  [CATALOG_TYPES.PLUGIN_RESOURCE]: (params) =>
    `/common/catalog/resources/${params.pluginId}/${params.slug}/${encodeURIComponent(params.instanceId)}`,
};

const APP_ID_FIELDS = {
  [CATALOG_TYPES.LLM]: "llm_ids",
  [CATALOG_TYPES.DATASOURCE]: "datasource_ids",
  [CATALOG_TYPES.TOOL]: "tool_ids",
  [CATALOG_TYPES.MCP_SERVER]: "mcp_server_ids",
  [CATALOG_TYPES.MODEL_ROUTER]: "model_router_ids",
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
  const rest = toolRestEnabled(attrs);
  const mcp = toolMcpEnabled(attrs);
  return (
    <>
    {/* Mirrors the MCP server page: who serves it, how you get in, where. A
        tool is listed here only if at least one method is on. */}
    <Section
      title="How to connect"
      description="This tool is served by AI Studio. Your app reaches it with its own credential."
    >
      <Grid container spacing={2}>
        <Field label="Access methods" md={4}>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }} data-testid="asset-tool-access">
            {rest && <Chip size="small" label="REST API" color="primary" />}
            {mcp && <Chip size="small" label="MCP" color="primary" />}
          </Box>
        </Field>
        <Field label="Authentication" md={8}>
          App credential as a bearer token{mcp ? ", or OAuth sign-in from an MCP client" : ""}
        </Field>
        {rest && attrs.rest_endpoint_url && (
          <Field label="REST endpoint" md={12}>
            <CopyableCode value={attrs.rest_endpoint_url} label="REST endpoint" />
          </Field>
        )}
        {mcp && attrs.mcp_endpoint_url && (
          <Field label="MCP endpoint" md={12}>
            <CopyableCode value={attrs.mcp_endpoint_url} label="MCP endpoint" />
          </Field>
        )}
      </Grid>
      <Typography variant="bodyMediumDefault" color="text.defaultSubdued" sx={{ mt: 2 }} data-testid="tool-access-note">
        Build an app with this tool. Once it is approved, the app page shows the credential and a ready-made MCP client configuration.
      </Typography>
    </Section>
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
    </>
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
        {external && (
          <>
            {" "}
            <RouterLink to={external}>Open it there</RouterLink>
            {granted ? " to see the full details." : " to see the details and request access."}
          </>
        )}
      </Typography>
    </Section>
  );
};

const MCPServerSections = ({ item }) => {
  const a = item.attributes || {};
  const primitives = a.primitives || [];
  const perTag = Object.entries(a.endpoint_urls || {});
  const oauth = a.oauth;
  return (
    <>
      <Section
        title="How to connect"
        description={
          a.brokerable
            ? "This server is served by a Tyk Gateway; AI Studio brokers access to it."
            : "This server is served by a Tyk Gateway; you connect to it directly."
        }
      >
        <Grid container spacing={2}>
          <Field label="Authentication" md={4}>
            {MCP_AUTH_LABELS[a.auth_mode] || a.auth_mode || "—"}
            {a.auth_header ? ` (${a.auth_header} header)` : ""}
          </Field>
          <Field label="Kind" md={4}>{kindLabel(item) || "—"}</Field>
          <Field label="Deployed to" md={4}>
            {(a.gateway_tags || []).length > 0 ? (
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }} data-testid="asset-gateway-tags">
                {a.gateway_tags.map((tag) => (
                  <Chip key={tag} size="small" label={tag} />
                ))}
              </Box>
            ) : (
              "All gateways"
            )}
          </Field>
          {a.endpoint_url && (
            <Field label="MCP endpoint" md={12}>
              <CopyableCode value={a.endpoint_url} label="MCP endpoint" />
            </Field>
          )}
          {perTag.map(([tag, url]) => (
            <Field key={tag} label={`MCP endpoint (${tag})`} md={12}>
              <CopyableCode value={url} label={`MCP endpoint for ${tag}`} />
            </Field>
          ))}
          {!a.endpoint_url && perTag.length === 0 && (
            <Field label="MCP endpoint" md={12}>
              Ask your platform team for the gateway address; it has not been configured in AI Studio.
            </Field>
          )}
          {oauth && (
            <Field label="OAuth authorization servers" md={12}>
              {(oauth.authorization_servers || []).length > 0 ? oauth.authorization_servers.join(", ") : "advertised by the server"}
              {oauth.url ? ` · metadata at ${oauth.url}` : ""}
            </Field>
          )}
        </Grid>
        <Typography variant="bodyMediumDefault" color="text.defaultSubdued" sx={{ mt: 2 }} data-testid="mcp-access-note">
          {a.brokerable
            ? "Build an app with this server and, once it is approved, request a Tyk access key from the app page."
            : a.auth_mode === "keyless"
              ? "AI Studio does not broker access to this server: no credential is needed, point your MCP client at the endpoint above."
              : "AI Studio does not broker access to this server: obtain a token from the authentication method above and point your MCP client at the endpoint."}
        </Typography>
      </Section>
      {/* "MCP tools": Tools are a different asset type in this catalog. */}
      <Section title="MCP tools, resources and prompts" description="What the server offers, as declared on the Tyk Gateway.">
        {primitives.length > 0 ? (
          <Box component="ul" sx={{ m: 0, pl: 2 }} data-testid="asset-primitives">
            {primitives.map((p) => (
              <li key={`${p.type}:${p.name}`}>
                <Typography variant="bodyMediumMedium" component="span">{p.name}</Typography>
                <Typography variant="bodySmallDefault" component="span" color="text.defaultSubdued">
                  {" "}· {p.type}
                  {p.description ? ` · ${p.description}` : ""}
                  {p.auth?.ignore_authentication ? " · no auth" : ""}
                </Typography>
              </li>
            ))}
          </Box>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            The definition does not list its primitives; connect a client to discover them.
          </Typography>
        )}
      </Section>
    </>
  );
};

// A chat completion against the unified ingress, the way an OpenAI SDK sends
// it. The app's secret is the bearer token, as on the app page.
const routerCurlSnippet = (baseUrl, model) =>
  [
    `curl ${baseUrl}/chat/completions \\`,
    `  -H "Authorization: Bearer $APP_SECRET" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '{"model": "${model}", "messages": [{"role": "user", "content": "Hello"}]}'`,
  ].join("\n");

const ModelRouterSections = ({ item }) => {
  const a = item.attributes || {};
  const models = a.router_models || [];
  const llms = a.router_llms || [];
  const baseUrl = unifiedIngressBaseUrl();
  const example = models[0] || `${a.router_slug || "<router>"}/<model>`;
  return (
    <>
      <Section
        title="Call it with"
        description="Apps granted this router call the gateway's unified OpenAI-compatible endpoint and name the router in the model field."
        data-testid="router-call-section"
      >
        {baseUrl ? (
          <>
            <FieldLabel variant="bodySmallDefault">Endpoint</FieldLabel>
            <CopyableCode value={`${baseUrl}/chat/completions`} label="endpoint" testId="router-endpoint" />
          </>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued" component="p" sx={{ m: 0 }}>
            Send requests to the gateway&apos;s unified endpoint, <code>POST /v1/chat/completions</code>. Ask your platform team for the gateway address.
          </Typography>
        )}
        <FieldLabel variant="bodySmallDefault" sx={{ mt: 2, display: "block" }}>
          Model names
        </FieldLabel>
        {models.length > 0 ? (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }} data-testid="router-models">
            {models.map((model) => (
              <CopyableCode key={model} value={model} label={`model ${model}`} />
            ))}
          </Box>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued" component="p" sx={{ m: 0 }}>
            The router matches model names by pattern. Send <code>{a.router_slug || "<router>"}/&lt;model&gt;</code> with a model its pools accept.
          </Typography>
        )}
        <Typography variant="bodySmallDefault" color="text.defaultSubdued" component="p" sx={{ mt: 1, mb: 2 }}>
          The part before the slash picks this router; the rest is the model it routes on.
        </Typography>
        <FieldLabel variant="bodySmallDefault">Example</FieldLabel>
        <CopyableBlock
          value={routerCurlSnippet(baseUrl || "https://<gateway>/v1", example)}
          label="example request"
          testId="router-example"
        />
        <Typography variant="bodySmallDefault" color="text.defaultSubdued" component="p" sx={{ mt: 1 }}>
          With an OpenAI SDK, set the base URL to the endpoint without <code>/chat/completions</code>, use the app secret as the API key and pass <code>{example}</code> as the model.
        </Typography>
      </Section>
      <Section
        title="Routes to"
        description="The LLM providers a request through this router may end up at. An app granted the router reaches them only through it."
      >
        {llms.length > 0 ? (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }} data-testid="router-llms">
            {llms.map((llm) => (
              <Chip
                key={llm.id}
                size="small"
                variant="outlined"
                label={llm.vendor ? `${llm.name} · ${getVendorName(llm.vendor) || llm.vendor}` : llm.name}
              />
            ))}
          </Box>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            No LLM providers are listed for this router.
          </Typography>
        )}
      </Section>
    </>
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
          {/* The providing plugin's page, when it declared one: the only way
              onward when the plugin manages access, a second view of the
              item when an App credential does. */}
          {secondaryActionPath(item) && (
            <SecondaryOutlineButton
              onClick={() => navigate(secondaryActionPath(item))}
              data-testid="asset-view-external"
            >
              {secondaryActionLabel(item)}
            </SecondaryOutlineButton>
          )}
          {isAppGranted(item) && (
            <PrimaryButton onClick={() => navigate(buildAppPath(item))} data-testid="asset-build-app">
              {buildActionLabel(item)}
            </PrimaryButton>
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
                  {type === CATALOG_TYPES.PLUGIN_RESOURCE || type === CATALOG_TYPES.MCP_SERVER ? "Your team's resource grants" : "—"}
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
        {type === CATALOG_TYPES.MCP_SERVER && <MCPServerSections item={item} />}
        {type === CATALOG_TYPES.MODEL_ROUTER && <ModelRouterSections item={item} />}
        {type === CATALOG_TYPES.PLUGIN_RESOURCE && <PluginResourceSections item={item} />}

        {governed.length > 0 && (
          <Section title="Governance" description="Metadata the organisation records about this asset.">
            <GovernedMetadataBadges items={governed} />
          </Section>
        )}

        {APP_ID_FIELDS[type] && isAppGranted(item) && (
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
