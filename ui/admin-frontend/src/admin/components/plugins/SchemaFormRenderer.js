import React from 'react';
import { Box, Alert, Typography } from '@mui/material';
import { Form } from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';

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
    ...(schema?.properties && Object.keys(schema.properties).reduce((acc, key) => {
      const property = schema.properties[key];

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

      return acc;
    }, {}))
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