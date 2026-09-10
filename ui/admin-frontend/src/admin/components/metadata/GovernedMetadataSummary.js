import React from 'react';
import { Box, Chip, Divider, Grid, Typography } from '@mui/material';
import { FieldLabel, FieldValue } from '../../styles/sharedStyles';
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
 * Read-only "Governance Metadata" block for admin detail pages.
 * Renders nothing in CE or when no schema fields apply.
 */
const GovernedMetadataSummary = ({ objectType, values, status, withDivider = true, title = 'Governance Metadata' }) => {
  const { fields, vocabulariesBySlug, usersById, loading } = useResolvedMetadataSchema(objectType);
  if (loading || fields.length === 0) return null;
  const data = values || {};

  return (
    <>
      {withDivider && <Divider sx={{ my: 3 }} />}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
        <Typography variant="h6">{title}</Typography>
        <MetadataStatusChip status={status} />
      </Box>
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
    </>
  );
};

export default GovernedMetadataSummary;
