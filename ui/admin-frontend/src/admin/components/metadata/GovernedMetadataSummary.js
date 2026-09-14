import React from 'react';
import { Box, Chip, Grid } from '@mui/material';
import { FieldLabel, FieldValue } from '../../styles/sharedStyles';
import Section from '../common/Section';
import useResolvedMetadataSchema from '../../hooks/useResolvedMetadataSchema';

const STATUS_COLOR = { valid: 'success', warnings: 'warning', invalid: 'error' };

export const MetadataStatusChip = ({ status }) => {
  if (!status) return null;
  return <Chip size="small" label={status} color={STATUS_COLOR[status] || 'default'} variant="outlined" />;
};

const renderValue = (field, raw, vocabulary, usersById) => {
  if (raw === undefined || raw === null || raw === '') return '—';
  switch (field.type) {
    case 'boolean':
      return raw ? 'Yes' : 'No';
    case 'user':
      return usersById[String(raw)] || `User ${raw}`;
    case 'vocabulary':
      return vocabulary?.terms?.find((t) => t.value === raw)?.label || String(raw);
    case 'multi_vocabulary': {
      const list = Array.isArray(raw) ? raw : [raw];
      return (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
          {list.map((v) => (
            <Chip key={v} size="small" label={vocabulary?.terms?.find((t) => t.value === v)?.label || v} />
          ))}
        </Box>
      );
    }
    case 'string_list': {
      const list = Array.isArray(raw) ? raw : [raw];
      return (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
          {list.map((v) => (
            <Chip key={v} size="small" label={v} />
          ))}
        </Box>
      );
    }
    default:
      return String(raw);
  }
};

/**
 * Read-only "Governance Metadata" section for admin detail pages, rendered
 * as one more bordered Section so it sits in the page like its neighbours
 * rather than as a loose block between them. Renders nothing in CE or when
 * no schema fields apply.
 */
const GovernedMetadataSummary = ({ objectType, values, status, title = 'Governance Metadata' }) => {
  const { fields, vocabulariesBySlug, usersById, loading } = useResolvedMetadataSchema(objectType);
  if (loading || fields.length === 0) return null;
  const data = values || {};

  return (
    <Section title={title} actions={<MetadataStatusChip status={status} />}>
      <Grid container spacing={2}>
        {fields.map((field) => (
          <React.Fragment key={field.key}>
            <Grid item xs={12} sm={3}>
              <FieldLabel>{field.label || field.key}:</FieldLabel>
            </Grid>
            <Grid item xs={12} sm={9}>
              <FieldValue component="div">
                {renderValue(field, data[field.key], vocabulariesBySlug[field.vocabulary_slug], usersById)}
              </FieldValue>
            </Grid>
          </React.Fragment>
        ))}
      </Grid>
    </Section>
  );
};

export default GovernedMetadataSummary;
