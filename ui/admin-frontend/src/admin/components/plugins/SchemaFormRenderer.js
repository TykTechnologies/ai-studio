import React from 'react';
import { Box, Alert, Typography } from '@mui/material';
import { Form } from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';

// A secret field: format "password", or writeOnly (the server never sends the
// value back). It must render masked whatever else applies to it.
export const isSecretProperty = (property) =>
  Boolean(property) && (property.format === 'password' || property.writeOnly === true);

// Per-field uiSchema for an object schema's properties, recursing into nested
// objects. Secret fields always get the password widget: the long-description
// textarea rule used to override format "password" and show secrets in clear.
export const fieldUiSchemas = (schema) => {
  const properties = schema?.properties;
  if (!properties) return {};
  return Object.keys(properties).reduce((acc, key) => {
    const property = properties[key] || {};

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
      const nested = fieldUiSchemas(property);
      if (Object.keys(nested).length > 0) acc[key] = { ...(acc[key] || {}), ...nested };
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