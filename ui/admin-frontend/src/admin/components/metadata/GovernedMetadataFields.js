import React, { useEffect, useMemo, useState } from 'react';
import {
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Chip,
  Grid,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useDebouncedCallback } from 'use-debounce';
import { StyledAccordion } from '../../styles/sharedStyles';
import { useEdition } from '../../context/EditionContext';
import useResolvedMetadataSchema from '../../hooks/useResolvedMetadataSchema';
import {
  validateObjectMetadata,
  validationResultToFieldMessages,
  normalizeGovernedMetadataValues,
} from '../../services/governedMetadataService';
import MetadataFieldInput from './inputs/MetadataFieldInput';

export const GOVERNED_METADATA_SECTION_ID = 'governed-metadata-section';

const statusChip = (enforcement) =>
  enforcement === 'enforce' ? (
    <Chip label="Enforced" size="small" color="error" variant="outlined" />
  ) : (
    <Chip label="Advisory" size="small" variant="outlined" />
  );

/**
 * Embeddable "Governance Metadata" section for the LLM / Tool / Datasource
 * forms. Self-gating: renders nothing in Community Edition or when no schema
 * fields apply to the object type.
 *
 * Props:
 *  - objectType: "llm" | "tool" | "datasource" | "plugin_resource:<id>:<slug>"
 *  - value: { [fieldKey]: any }
 *  - onChange(nextValues)
 *  - errors: { [fieldKey]: message } from a 422 response ("_" for non-field errors)
 */
const GovernedMetadataFields = ({ objectType, value, onChange, errors = {}, defaultExpanded = false }) => {
  const { isEnterprise } = useEdition();
  const { fields, vocabulariesBySlug, users, enforcement, loading } = useResolvedMetadataSchema(objectType);
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [live, setLive] = useState({ errors: {}, warnings: {} });
  const values = useMemo(() => value || {}, [value]);

  const hasServerErrors = Object.keys(errors || {}).length > 0;
  const hasLiveErrors = Object.keys(live.errors).length > 0;

  useEffect(() => {
    if (enforcement === 'enforce' || hasServerErrors || hasLiveErrors) {
      setExpanded(true);
    }
  }, [enforcement, hasServerErrors, hasLiveErrors]);

  const runValidation = useDebouncedCallback(async (nextValues) => {
    try {
      const result = await validateObjectMetadata(objectType, nextValues);
      setLive(validationResultToFieldMessages(result));
    } catch (e) {
      // Live validation is advisory; the server validates again on save.
    }
  }, 500);

  if (!isEnterprise || loading || fields.length === 0) {
    return null;
  }

  const handleFieldChange = (key, nextValue) => {
    const next = { ...values };
    if (nextValue === undefined) {
      delete next[key];
    } else {
      next[key] = nextValue;
    }
    const normalized = normalizeGovernedMetadataValues(next);
    onChange(normalized);
    runValidation(normalized);
  };

  return (
    <StyledAccordion
      id={GOVERNED_METADATA_SECTION_ID}
      expanded={expanded}
      onChange={(e, isExpanded) => setExpanded(isExpanded)}
      sx={{ mt: 2 }}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          <Typography variant="h6">Governance Metadata</Typography>
          <Chip label="Enterprise" size="small" color="primary" />
          {statusChip(enforcement)}
        </Box>
      </AccordionSummary>
      <AccordionDetails>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Ownership, lifecycle and classification information defined by your metadata schemas.
          {enforcement === 'enforce'
            ? ' Required fields must be valid before this object can be saved.'
            : ' Issues are reported but do not block saving.'}
        </Typography>
        {errors._ && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {errors._}
          </Alert>
        )}
        <Grid container spacing={3}>
          {fields.map((field) => {
            const wide = field.type === 'text' || field.type === 'string_list';
            return (
              <Grid item xs={12} md={wide ? 12 : 6} key={field.key}>
                <MetadataFieldInput
                  field={field}
                  value={values[field.key]}
                  onChange={(next) => handleFieldChange(field.key, next)}
                  error={errors[field.key] || live.errors[field.key]}
                  warning={live.warnings[field.key]}
                  vocabulary={vocabulariesBySlug[field.vocabulary_slug]}
                  users={users}
                  enforced={enforcement === 'enforce'}
                />
              </Grid>
            );
          })}
        </Grid>
      </AccordionDetails>
    </StyledAccordion>
  );
};

export default GovernedMetadataFields;
