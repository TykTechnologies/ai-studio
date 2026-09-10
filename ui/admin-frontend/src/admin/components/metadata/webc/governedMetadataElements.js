import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import createCache from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { ThemeProvider } from '@mui/material/styles';
import { Alert, Box, Chip, Grid, Typography } from '@mui/material';
import theme from '../../../theme';
import MetadataFieldInput from '../inputs/MetadataFieldInput';
import GovernedMetadataBadges from '../../../../portal/components/GovernedMetadataBadges';
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
  validationResultToFieldMessages,
  normalizeGovernedMetadataValues,
} from '../../../services/governedMetadataService';

/**
 * Host-provided Web Components so plugin UIs (which are Web Components loaded
 * into this document, usually with their own shadow DOM) can embed the standard
 * Governance Metadata form and badges instead of re-implementing them:
 *
 *   <governed-metadata-fields object-type="plugin_resource:12:prompts"></governed-metadata-fields>
 *   <governed-metadata-badges></governed-metadata-badges>   (element.items = display list)
 *
 * Each element renders into its own shadow root with a scoped Emotion cache, so
 * MUI styling works wherever the element is placed. See docs/site/docs/governed-metadata.md.
 */

export const GOVERNED_METADATA_FIELDS_TAG = 'governed-metadata-fields';
export const GOVERNED_METADATA_BADGES_TAG = 'governed-metadata-badges';

const parseJSON = (raw, fallback) => {
  if (raw === null || raw === undefined || raw === '') return fallback;
  if (typeof raw !== 'string') return raw;
  try {
    return JSON.parse(raw);
  } catch (e) {
    return fallback;
  }
};

/** Normalises a resolved schema payload (host REST or plugin-provided) to what the inputs need. */
const normalizeSchema = (schema) => {
  if (!schema) return null;
  const fields = [...(schema.fields || [])].sort((a, b) => (a.order || 0) - (b.order || 0));
  const vocabulariesBySlug = {};
  const rawVocab = schema.vocabularies || schema.vocabulariesBySlug || {};
  if (Array.isArray(rawVocab)) {
    rawVocab.forEach((v) => {
      vocabulariesBySlug[v.slug] = v;
    });
  } else {
    Object.entries(rawVocab).forEach(([slug, v]) => {
      vocabulariesBySlug[slug] = Array.isArray(v) ? { slug, terms: v } : v;
    });
  }
  return {
    fields,
    vocabulariesBySlug,
    users: schema.users || [],
    enforcement: schema.enforcement || 'advisory',
  };
};

/** Loads the schema through the admin REST API (requires an admin session). */
const loadSchemaFromHost = async (objectType) => {
  const resolved = await resolveMetadataSchema(objectType);
  const fields = resolved?.fields || [];
  if (fields.length === 0) {
    return normalizeSchema({ fields: [], enforcement: resolved?.enforcement });
  }
  const needsVocab = !resolved.vocabularies && fields.some((f) => f.type === 'vocabulary' || f.type === 'multi_vocabulary');
  const needsUsers = fields.some((f) => f.type === 'user');
  const [vocabularies, users] = await Promise.all([
    needsVocab ? getMetadataVocabularies() : Promise.resolve(resolved.vocabularies || []),
    needsUsers ? getMetadataUsers() : Promise.resolve([]),
  ]);
  return normalizeSchema({ fields, vocabularies, users, enforcement: resolved.enforcement });
};

const FieldsApp = ({ host, objectType, providedSchema, value, externalErrors, disabled }) => {
  const [schema, setSchema] = useState(() => normalizeSchema(providedSchema));
  const [loading, setLoading] = useState(!providedSchema && Boolean(objectType));
  const [loadError, setLoadError] = useState(null);
  const [values, setValues] = useState(value || {});
  const [live, setLive] = useState({ errors: {}, warnings: {} });
  const validateTimer = useRef(null);
  const canValidateOnHost = !providedSchema;

  useEffect(() => {
    setValues(value || {});
  }, [value]);

  useEffect(() => {
    let cancelled = false;
    if (providedSchema) {
      const next = normalizeSchema(providedSchema);
      setSchema(next);
      setLoading(false);
      host.dispatchEvent(new CustomEvent('ready', { detail: { fields: next.fields, enforcement: next.enforcement }, bubbles: true, composed: true }));
      return undefined;
    }
    if (!objectType) {
      setSchema(null);
      setLoading(false);
      return undefined;
    }
    setLoading(true);
    setLoadError(null);
    loadSchemaFromHost(objectType)
      .then((next) => {
        if (cancelled) return;
        setSchema(next);
        setLoading(false);
        host.dispatchEvent(new CustomEvent('ready', { detail: { fields: next.fields, enforcement: next.enforcement }, bubbles: true, composed: true }));
      })
      .catch((error) => {
        if (cancelled) return;
        setSchema(null);
        setLoading(false);
        setLoadError(error?.response?.status === 403 ? 'Governance metadata is an Enterprise feature.' : 'Could not load the governance metadata schema.');
        host.dispatchEvent(new CustomEvent('error', { detail: { error }, bubbles: true, composed: true }));
      });
    return () => {
      cancelled = true;
    };
  }, [objectType, providedSchema, host]);

  const fields = useMemo(() => schema?.fields || [], [schema]);
  const enforcement = schema?.enforcement || 'advisory';
  const errors = useMemo(() => ({ ...(live.errors || {}), ...(externalErrors || {}) }), [live.errors, externalErrors]);

  useEffect(() => {
    host._current = { values, fields, enforcement, objectType };
  }, [host, values, fields, enforcement, objectType]);

  const runLiveValidation = (next) => {
    if (!canValidateOnHost || !objectType) return;
    if (validateTimer.current) clearTimeout(validateTimer.current);
    validateTimer.current = setTimeout(async () => {
      try {
        const result = await validateObjectMetadata(objectType, next);
        setLive(validationResultToFieldMessages(result));
      } catch (e) {
        // Live validation is advisory; the plugin validates again on save.
      }
    }, 400);
  };

  const handleFieldChange = (key, nextValue) => {
    const next = { ...values };
    if (nextValue === undefined) {
      delete next[key];
    } else {
      next[key] = nextValue;
    }
    const normalized = normalizeGovernedMetadataValues(next);
    setValues(normalized);
    host.dispatchEvent(new CustomEvent('change', { detail: { values: normalized, objectType }, bubbles: true, composed: true }));
    runLiveValidation(normalized);
  };

  if (loading) {
    return null;
  }
  if (loadError) {
    return <Alert severity="warning">{loadError}</Alert>;
  }
  if (fields.length === 0) {
    return null;
  }

  return (
    <Box data-testid="governed-metadata-fields">
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Typography variant="subtitle1">Governance Metadata</Typography>
        {enforcement === 'enforce' ? (
          <Chip label="Enforced" size="small" color="error" variant="outlined" />
        ) : (
          <Chip label="Advisory" size="small" variant="outlined" />
        )}
      </Box>
      {externalErrors?._ && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {externalErrors._}
        </Alert>
      )}
      <Grid container spacing={2}>
        {fields.map((field) => {
          const wide = field.type === 'text' || field.type === 'string_list';
          return (
            <Grid item xs={12} md={wide ? 12 : 6} key={field.key}>
              <MetadataFieldInput
                field={field}
                value={values[field.key]}
                onChange={(next) => handleFieldChange(field.key, next)}
                error={errors[field.key]}
                warning={live.warnings?.[field.key]}
                vocabulary={schema.vocabulariesBySlug[field.vocabulary_slug]}
                users={schema.users}
                enforced={enforcement === 'enforce'}
                disabled={disabled}
              />
            </Grid>
          );
        })}
      </Grid>
    </Box>
  );
};

/** Shared plumbing: shadow root + scoped Emotion cache + React root. */
class ReactShadowElement extends HTMLElement {
  constructor() {
    super();
    this._shadow = this.attachShadow({ mode: 'open' });
    this._mount = document.createElement('div');
    this._shadow.appendChild(this._mount);
    this._cache = createCache({ key: 'governed-metadata', container: this._shadow, prepend: true });
    this._root = null;
  }

  connectedCallback() {
    if (!this._root) {
      this._root = createRoot(this._mount);
    }
    this._render();
  }

  disconnectedCallback() {
    if (this._root) {
      const root = this._root;
      this._root = null;
      // Defer so React never unmounts synchronously inside a DOM mutation callback
      // (a plugin re-rendering its own tree removes us mid-render). Swallow errors:
      // the element may be gone for good and nothing can act on them.
      Promise.resolve().then(() => {
        try {
          root.unmount();
        } catch (e) {
          // ignore
        }
      });
    }
  }

  attributeChangedCallback() {
    this._render();
  }

  _renderTree(tree) {
    if (!this._root) return;
    this._root.render(
      <CacheProvider value={this._cache}>
        <ThemeProvider theme={theme}>{tree}</ThemeProvider>
      </CacheProvider>
    );
  }
}

/**
 * <governed-metadata-fields>
 *
 * Properties / attributes:
 *  - objectType / object-type: "llm" | "tool" | "datasource" | "plugin_resource:<id>:<slug>".
 *    When no `schema` is provided the element loads the schema, vocabularies and
 *    users through the admin REST API (admin session required) and validates live.
 *  - schema: resolved schema payload ({fields, vocabularies, enforcement, users?}) as
 *    returned by the plugin management API (GetResolvedMetadataSchema). Use it from
 *    portal pages or wherever the admin API is not reachable; validation then happens
 *    on save through the plugin (SetObjectMetadata) and errors come back via `errors`.
 *  - value: current values object (or JSON string attribute).
 *  - errors: { [fieldKey]: message, _: "form-level message" } from a failed save.
 *  - disabled: boolean attribute.
 * Events: `ready` ({fields, enforcement}), `change` ({values, objectType}), `error`.
 * Methods: getValues(), setErrors(obj), validate() → validation result (admin API).
 */
class GovernedMetadataFieldsElement extends ReactShadowElement {
  static get observedAttributes() {
    return ['object-type', 'value', 'errors', 'disabled', 'schema'];
  }

  constructor() {
    super();
    this._props = { value: undefined, errors: undefined, schema: undefined, objectType: undefined };
    this._current = { values: {}, fields: [], enforcement: 'advisory' };
  }

  get objectType() {
    return this._props.objectType ?? this.getAttribute('object-type') ?? undefined;
  }

  set objectType(v) {
    this._props.objectType = v || undefined;
    this._render();
  }

  get value() {
    return this._props.value ?? parseJSON(this.getAttribute('value'), undefined);
  }

  set value(v) {
    this._props.value = v && typeof v === 'object' ? { ...v } : parseJSON(v, undefined);
    this._render();
  }

  get errors() {
    return this._props.errors ?? parseJSON(this.getAttribute('errors'), undefined);
  }

  set errors(v) {
    this._props.errors = v && typeof v === 'object' ? { ...v } : parseJSON(v, undefined);
    this._render();
  }

  get schema() {
    return this._props.schema ?? parseJSON(this.getAttribute('schema'), undefined);
  }

  set schema(v) {
    this._props.schema = v && typeof v === 'object' ? v : parseJSON(v, undefined);
    this._render();
  }

  getValues() {
    return { ...(this._current?.values || {}) };
  }

  setErrors(errors) {
    this.errors = errors;
  }

  async validate() {
    const objectType = this.objectType;
    if (!objectType) {
      throw new Error('governed-metadata-fields: object-type is required to validate');
    }
    const result = await validateObjectMetadata(objectType, this.getValues());
    const messages = validationResultToFieldMessages(result);
    this.errors = messages.errors;
    return result;
  }

  _render() {
    this._renderTree(
      <FieldsApp
        host={this}
        objectType={this.objectType}
        providedSchema={this.schema}
        value={this.value}
        externalErrors={this.errors}
        disabled={this.hasAttribute('disabled')}
      />
    );
  }
}

/**
 * <governed-metadata-badges>
 * Property / attribute `items`: the portal display list [{key,label,type,value}]
 * (GetObjectMetadata with visibility "portal" → display_json, or the
 * `governed_metadata` array on portal REST responses).
 */
class GovernedMetadataBadgesElement extends ReactShadowElement {
  static get observedAttributes() {
    return ['items'];
  }

  constructor() {
    super();
    this._items = undefined;
  }

  get items() {
    return this._items ?? parseJSON(this.getAttribute('items'), []);
  }

  set items(v) {
    this._items = Array.isArray(v) ? v : parseJSON(v, []);
    this._render();
  }

  _render() {
    this._renderTree(<GovernedMetadataBadges items={this.items} />);
  }
}

/** Defines both elements once; safe to call repeatedly. */
export const registerGovernedMetadataElements = () => {
  if (typeof window === 'undefined' || !window.customElements) return;
  if (!customElements.get(GOVERNED_METADATA_FIELDS_TAG)) {
    customElements.define(GOVERNED_METADATA_FIELDS_TAG, GovernedMetadataFieldsElement);
  }
  if (!customElements.get(GOVERNED_METADATA_BADGES_TAG)) {
    customElements.define(GOVERNED_METADATA_BADGES_TAG, GovernedMetadataBadgesElement);
  }
};

export default registerGovernedMetadataElements;
