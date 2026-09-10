import React from 'react';
import {
  Chip,
  FormControl,
  FormControlLabel,
  FormHelperText,
  InputLabel,
  MenuItem,
  Select,
  Switch,
  TextField,
} from '@mui/material';
import StringListInput from './StringListInput';

const helperStyle = (warning) => (warning ? { sx: { color: 'warning.main' } } : undefined);

const todayISO = () => new Date().toISOString().slice(0, 10);

/**
 * Renders the right input for a governed metadata field definition.
 * Controlled: value in, onChange(nextValue) out. Empty values are emitted as
 * undefined so the parent can drop the key.
 */
const MetadataFieldInput = ({
  field,
  value,
  onChange,
  error,
  warning,
  disabled = false,
  vocabulary,
  users = [],
  enforced = false,
}) => {
  const id = `governed-metadata-field-${field.key}`;
  const label = field.label || field.key;
  const helper = error || warning || field.description || '';
  // The browser's own constraint validation must only block the form when the
  // server would reject the save too: an enforced schema and an error-severity
  // field. Advisory schemas and warning-severity fields report, never block.
  const blocking = enforced && Boolean(field.required) && (field.severity || 'error') === 'error';
  const common = {
    id,
    label,
    required: blocking,
    disabled,
    error: Boolean(error),
    helperText: helper,
    FormHelperTextProps: !error && warning ? helperStyle(true) : undefined,
    fullWidth: true,
  };

  switch (field.type) {
    case 'text':
      return (
        <TextField
          {...common}
          multiline
          rows={3}
          value={value ?? ''}
          inputProps={{ maxLength: field.max_length || undefined }}
          onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
        />
      );

    case 'number':
      return (
        <TextField
          {...common}
          type="number"
          value={value ?? ''}
          inputProps={{ min: field.min ?? undefined, max: field.max ?? undefined, step: 'any' }}
          onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
        />
      );

    case 'boolean':
      return (
        <FormControl error={Boolean(error)} disabled={disabled}>
          <FormControlLabel
            control={<Switch id={id} checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />}
            label={label}
          />
          {helper && <FormHelperText {...helperStyle(!error && warning)}>{helper}</FormHelperText>}
        </FormControl>
      );

    case 'date': {
      const localWarning =
        !error && !warning && field.warn_if_past && value && value < todayISO() ? `${label} is in the past` : undefined;
      return (
        <TextField
          {...common}
          type="date"
          value={value ?? ''}
          helperText={error || warning || localWarning || field.description || ''}
          FormHelperTextProps={!error && (warning || localWarning) ? helperStyle(true) : undefined}
          InputLabelProps={{ shrink: true }}
          onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
        />
      );
    }

    case 'user':
      return (
        <FormControl fullWidth error={Boolean(error)} disabled={disabled} required={blocking}>
          <InputLabel id={`${id}-label`}>{label}</InputLabel>
          <Select
            labelId={`${id}-label`}
            id={id}
            label={label}
            value={value ?? ''}
            onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
          >
            {!field.required && (
              <MenuItem value="">
                <em>None</em>
              </MenuItem>
            )}
            {users.map((u) => {
              const attrs = u.attributes || u;
              return (
                <MenuItem key={u.id} value={Number(u.id)}>
                  {attrs.name ? `${attrs.name} (${attrs.email})` : attrs.email}
                </MenuItem>
              );
            })}
          </Select>
          {helper && <FormHelperText {...helperStyle(!error && warning)}>{helper}</FormHelperText>}
        </FormControl>
      );

    case 'vocabulary':
    case 'multi_vocabulary': {
      const multiple = field.type === 'multi_vocabulary';
      const terms = vocabulary?.terms || [];
      const labelFor = (v) => terms.find((t) => t.value === v)?.label || v;
      const selected = multiple ? (Array.isArray(value) ? value : []) : value ?? '';
      return (
        <FormControl fullWidth error={Boolean(error)} disabled={disabled} required={blocking}>
          <InputLabel id={`${id}-label`}>{label}</InputLabel>
          <Select
            labelId={`${id}-label`}
            id={id}
            label={label}
            multiple={multiple}
            value={selected}
            renderValue={
              multiple
                ? (vals) => (
                    <span style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {vals.map((v) => (
                        <Chip key={v} label={labelFor(v)} size="small" />
                      ))}
                    </span>
                  )
                : undefined
            }
            onChange={(e) => {
              const next = e.target.value;
              if (multiple) {
                onChange(Array.isArray(next) && next.length > 0 ? next : undefined);
              } else {
                onChange(next === '' ? undefined : next);
              }
            }}
          >
            {!multiple && !field.required && (
              <MenuItem value="">
                <em>None</em>
              </MenuItem>
            )}
            {terms.map((t) => {
              const isSelected = multiple ? selected.includes(t.value) : selected === t.value;
              return (
                <MenuItem key={t.value} value={t.value} disabled={t.deprecated && !isSelected}>
                  {t.label || t.value}
                  {t.deprecated ? ' (deprecated)' : ''}
                </MenuItem>
              );
            })}
          </Select>
          {helper && <FormHelperText {...helperStyle(!error && warning)}>{helper}</FormHelperText>}
        </FormControl>
      );
    }

    case 'string_list':
      return (
        <StringListInput
          id={id}
          label={label}
          value={Array.isArray(value) ? value : []}
          required={blocking}
          disabled={disabled}
          error={Boolean(error)}
          helperText={helper}
          onChange={(next) => onChange(next.length > 0 ? next : undefined)}
        />
      );

    case 'email':
    case 'url':
    case 'string':
    default:
      return (
        <TextField
          {...common}
          type={field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : 'text'}
          value={value ?? ''}
          inputProps={{ maxLength: field.max_length || undefined }}
          onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
        />
      );
  }
};

export default MetadataFieldInput;
