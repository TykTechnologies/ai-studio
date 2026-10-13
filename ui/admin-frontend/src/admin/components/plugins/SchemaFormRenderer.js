import React from 'react';
import { Box, Alert, Typography } from '@mui/material';
import { Form } from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';

// A secret field: format "password", or writeOnly. It must render masked
// whatever else applies to it. Masking is display-only: the plugin config API
// still returns stored values to the form.
export const isSecretProperty = (property) =>
  Boolean(property) && (property.format === 'password' || property.writeOnly === true);

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

const SchemaFormRenderer = ({
  schema,
  formData,
  onChange,
  onError,
  disabled = false
}) => {
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
    ...fieldUiSchemas(schema),
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
        {/* Schema Form */}
        <Form
          schema={schema}
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