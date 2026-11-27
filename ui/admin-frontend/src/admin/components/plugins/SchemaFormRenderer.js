import React from 'react';
import { Box, Alert, Typography } from '@mui/material';
import { Form } from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';

// A secret field: format "password", or writeOnly. It must render masked
// whatever else applies to it. The plugin config API marks the same fields
// (services/plugin_config_redaction.go) and returns a stored secret as
// REDACTED_VALUE, never the value; sending that marker back keeps it.
export const isSecretProperty = (property) =>
  Boolean(property) && (property.format === 'password' || property.writeOnly === true);

// What the API returns in place of a stored secret.
export const REDACTED_VALUE = '[redacted]';

// hasRedactedValue reports whether a configuration holds a stored secret the
// API did not return, at any depth.
export const hasRedactedValue = (value) => {
  if (value === REDACTED_VALUE) return true;
  if (Array.isArray(value)) return value.some(hasRedactedValue);
  if (value && typeof value === 'object') return Object.values(value).some(hasRedactedValue);
  return false;
};

// Follows a local "$ref" ("#/definitions/X", "#/$defs/X") against the root
// schema, as RJSF does, so a secret declared in a definition is still seen.
// Unresolvable or remote refs, and ref chains longer than a few hops (or
// cycles), leave the property as it is.
const MAX_REF_HOPS = 8;
// resolveRefTrail is resolveRef that also returns the refs it followed.
const resolveRefTrail = (property, root) => {
  const refs = [];
  let current = property;
  for (let hop = 0; hop < MAX_REF_HOPS && current && typeof current.$ref === 'string'; hop += 1) {
    if (!current.$ref.startsWith('#/')) return { schema: current, refs };
    const target = current.$ref
      .slice(2)
      .split('/')
      .map((part) => part.replace(/~1/g, '/').replace(/~0/g, '~'))
      .reduce((node, part) => (node && typeof node === 'object' ? node[part] : undefined), root);
    if (!target) return { schema: current, refs };
    refs.push(current.$ref);
    const { $ref, ...siblings } = current;
    current = { ...target, ...siblings };
  }
  return { schema: current, refs };
};
export const resolveRef = (property, root) => resolveRefTrail(property, root).schema;

// Per-field uiSchema for an object schema's properties, recursing into nested
// objects, array items and local $refs. Secret fields always get the password
// widget: the long-description textarea rule used to override format
// "password" and show secrets in clear.
//
// A recursive definition (Node.child -> #/definitions/Node) is walked once:
// a $ref already followed on the current path is not expanded again, which
// keeps the cost linear in the schema size even when every property of a
// definition refers back to it. Deeper copies still mask format "password"
// natively in RJSF. Each definition's uiSchema is built once and reused
// (memo), so a chain of definitions that each refer several times to the
// next stays linear too. MAX_DEPTH is a backstop for deep inline schemas.
const MAX_DEPTH = 8;
export const fieldUiSchemas = (schema, root = schema, depth = 0, path = new Set(), memo = new Map()) => {
  if (depth > MAX_DEPTH) return {};
  const self = resolveRefTrail(schema, root);
  const onPath = new Set([...path, ...self.refs]);
  const properties = self.schema?.properties;
  if (!properties) return {};
  // Expands a child schema, unless it refers back to a definition on the
  // path, reusing an earlier expansion of the same definition.
  const expand = (trail) => {
    if (trail.refs.some((ref) => onPath.has(ref))) return {};
    const key = trail.refs[trail.refs.length - 1];
    if (key && memo.has(key)) return memo.get(key);
    const built = fieldUiSchemas(trail.schema, root, depth + 1, new Set([...onPath, ...trail.refs]), memo);
    if (key) memo.set(key, built);
    return built;
  };
  return Object.keys(properties).reduce((acc, key) => {
    const trail = resolveRefTrail(properties[key] || {}, root);
    const property = trail.schema;

    if (isSecretProperty(property)) {
      acc[key] = { "ui:widget": "password" };
      return acc;
    }

    // URL fields get better input validation
    if (property.format === 'uri') {
      acc[key] = {
        "ui:help": property.examples ? `Examples: ${property.examples.join(', ')}` : undefined,
      };
    }

    // Numeric fields get better input controls
    if (property.type === 'number' || property.type === 'integer') {
      acc[key] = {
        "ui:widget": "updown",
      };
    }

    // Long descriptions become textareas
    if (property.type === 'string' && property.description && property.description.length > 100) {
      acc[key] = {
        "ui:widget": "textarea",
        "ui:options": {
          rows: 3,
        },
      };
    }

    // Nested objects get the same treatment, so a secret inside one is
    // masked too.
    if (property.type === 'object' && property.properties) {
      const nested = expand(trail);
      if (Object.keys(nested).length > 0) acc[key] = { ...(acc[key] || {}), ...nested };
    }

    // Arrays: RJSF reads the item uiSchema from the key's "items", for an
    // array of secret strings and for secrets inside array-of-object items.
    if (property.type === 'array' && property.items && !Array.isArray(property.items)) {
      const itemTrail = resolveRefTrail(property.items, root);
      const items = isSecretProperty(itemTrail.schema)
        ? { "ui:widget": "password" }
        : expand(itemTrail);
      if (Object.keys(items).length > 0) acc[key] = { ...(acc[key] || {}), items };
    }

    return acc;
  }, {});
};

// RJSF renders properties and additionalProperties, never patternProperties.
// A plugin that keeps settings at the top level under names the user picks
// declares them with patternProperties (Prompt Compressor's
// engine_<name>_url and engine_<name>_token: top level, so $SECRET/
// references resolve), and the form showed no field for them. This exposes
// them: each key of the configuration that matches a pattern becomes a
// property with that pattern's schema, so it renders, validates and masks
// like a declared one. When the schema closes the object
// (additionalProperties false), keys can also be added, and propertyNames
// keeps a new key's name to the declared properties and the patterns. An
// open schema keeps its own additionalProperties. A pattern that does not
// compile is ignored.
const compilePatterns = (patternProperties) =>
  Object.entries(patternProperties).flatMap(([source, subschema]) => {
    try {
      return [{ source, re: new RegExp(source, 'u'), subschema: subschema || {} }];
    } catch (e) {
      return [];
    }
  });

export const withPatternProperties = (schema, formData) => {
  const patternProperties = schema?.patternProperties;
  if (!patternProperties || typeof patternProperties !== 'object') return schema;
  const patterns = compilePatterns(patternProperties);
  if (patterns.length === 0) return schema;

  const properties = { ...(schema.properties || {}) };
  const data = formData && typeof formData === 'object' && !Array.isArray(formData) ? formData : {};
  Object.keys(data).forEach((key) => {
    if (properties[key]) return;
    const match = patterns.find(({ re }) => re.test(key));
    if (match) properties[key] = { title: key, ...match.subschema };
  });

  if (schema.additionalProperties !== false) return { ...schema, properties };

  const subschemas = patterns.map(({ subschema }) => subschema);
  return {
    ...schema,
    properties,
    additionalProperties: subschemas.length === 1 ? subschemas[0] : { anyOf: subschemas },
    propertyNames: {
      anyOf: [
        { enum: Object.keys(properties) },
        ...patterns.map(({ source }) => ({ pattern: source })),
      ],
    },
  };
};

// A key added under additionalProperties is masked when any pattern marks
// its values secret: RJSF has one uiSchema for all added keys.
const additionalUiSchema = (schema) => {
  const patternProperties = schema?.patternProperties;
  if (schema?.additionalProperties !== false || !patternProperties) return {};
  const secret = Object.values(patternProperties).some((sub) => isSecretProperty(resolveRef(sub || {}, schema)));
  return secret ? { additionalProperties: { "ui:widget": "password" } } : {};
};

const SchemaFormRenderer = ({
  schema,
  formData,
  onChange,
  onError,
  disabled = false
}) => {
  const formSchema = withPatternProperties(schema, formData);

  // Custom UI Schema for better Material-UI integration
  const uiSchema = {
    "ui:submitButtonOptions": {
      "props": {
        "style": { display: "none" }  // Hide the default submit button
      }
    },
    "ui:globalOptions": {
      "copyable": false,  // Disable copy functionality
      "label": true,      // Show labels
    },
    // Customize specific field types for better UX
    ...fieldUiSchemas(formSchema),
    ...additionalUiSchema(schema),
  };

  const handleFormChange = ({ formData: newFormData }) => {
    onChange(newFormData);
  };

  const handleFormError = (errors) => {
    if (onError) {
      onError(errors);
    }
    console.error('Form validation errors:', errors);
  };

  // Validate schema before rendering
  if (!schema) {
    return (
      <Alert severity="info">
        <Typography variant="body2">
          No configuration schema available for this plugin. Using JSON editor.
        </Typography>
      </Alert>
    );
  }

  // Check for basic schema validity
  if (!schema.type || schema.type !== 'object') {
    return (
      <Alert severity="warning">
        <Typography variant="body2">
          Invalid schema format received from plugin. Using JSON editor.
        </Typography>
      </Alert>
    );
  }

  try {
    return (
      <Box>
        {hasRedactedValue(formData) && (
          <Alert severity="info" sx={{ mb: 2 }} data-testid="plugin-config-redacted-note">
            <Typography variant="body2">
              Stored secrets are not shown. A secret field left as it is keeps its stored value; type a new
              value to replace it, or clear the field to remove it.
            </Typography>
          </Alert>
        )}
        {/* Schema Form */}
        <Form
          schema={formSchema}
          uiSchema={uiSchema}
          formData={formData}
          onChange={handleFormChange}
          onError={handleFormError}
          validator={validator}
          disabled={disabled}
          showErrorList={false}  // We'll handle errors in the parent
          liveValidate={true}    // Real-time validation
        />

        {/* Schema Metadata (development only) */}
        {process.env.NODE_ENV === 'development' && schema.title && (
          <Box mt={2} p={1} bgcolor="grey.50" borderRadius={1}>
            <Typography variant="caption" color="textSecondary">
              <strong>Schema:</strong> {schema.title}
              {schema.description && ` - ${schema.description}`}
              <br />
              <strong>Properties:</strong> {Object.keys(schema.properties || {}).length}
              {schema.required && (
                <>
                  <br />
                  <strong>Required:</strong> {schema.required.join(', ')}
                </>
              )}
            </Typography>
          </Box>
        )}
      </Box>
    );
  } catch (error) {
    console.error('Error rendering schema form:', error);

    return (
      <Alert severity="error">
        <Typography variant="body2">
          Failed to render configuration form: {error.message}
        </Typography>
        <Typography variant="caption" color="textSecondary" display="block" mt={1}>
          Please use the JSON editor instead.
        </Typography>
      </Alert>
    );
  }
};

export default SchemaFormRenderer;