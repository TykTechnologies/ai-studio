import { styled } from "@mui/material/styles";
import { Box, Typography, Stack, IconButton } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import React from "react";

export const SectionContainer = styled(Stack)(() => ({
  spacing: 2,
}));

export const FieldRow = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'flex-start',
  width: '100%',
  [theme.breakpoints.down('sm')]: {
    flexDirection: 'column',
    marginBottom: theme.spacing(2),
  },
}));

export const FieldGroup = styled(Box)(({ theme, width = '50%' }) => ({
  display: 'flex',
  alignItems: 'flex-start',
  width: width,
  [theme.breakpoints.down('md')]: {
    width: '100%',
    marginBottom: theme.spacing(2),
  },
  [theme.breakpoints.down('sm')]: {
    flexDirection: 'column',
  },
}));

export const TwoColumnLayout = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'flex-start',
  width: '100%',
  flexDirection: 'row',
  [theme.breakpoints.down('md')]: {
    flexDirection: 'column',
    '& > *:not(:last-child)': {
      marginBottom: theme.spacing(2),
    },
  },
}));

export const FieldLabel = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.primary,
  minWidth: '20%',
  marginRight: theme.spacing(2),
  [theme.breakpoints.down('md')]: {
    minWidth: '40%',
  },
  [theme.breakpoints.down('sm')]: {
    width: '100% !important',
    marginBottom: theme.spacing(1),
  },
}));

export const FieldValue = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.defaultSubdued,
  wordBreak: 'break-word',
  overflowWrap: 'break-word',
  [theme.breakpoints.down('sm')]: {
    marginLeft: '0 !important',
    width: '100%',
  },
}));

export const BreakableFieldValue = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.defaultSubdued,
  wordBreak: 'break-word',
  overflowWrap: 'break-word',
  maxWidth: '60%',
  [theme.breakpoints.down('md')]: {
    maxWidth: '100%',
  },
  [theme.breakpoints.down('sm')]: {
    marginLeft: '0 !important',
    width: '100%',
  },
}));

export const CopyIcon = styled(ContentCopyIcon)(({ theme }) => ({
  color: theme.palette.text.defaultSubdued,
  width: 16,
  height: 16,
}));

export const CopyButton = styled(IconButton)(({ theme }) => ({
  marginLeft: theme.spacing(1),
  marginTop: theme.spacing(-0.3),
}));

export const CopyableFieldContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'flex-start',
  marginLeft: 1,
  flexGrow: 1,
  wordBreak: 'break-word',
  overflowWrap: 'break-word',
  [theme.breakpoints.down('sm')]: {
    marginLeft: 0,
    width: '100%',
  },
}));

export const CopyableField = ({ value, fieldName, handleCopyToClipboard }) => {
  return (
    <CopyableFieldContainer>
      <FieldValue variant="bodyLargeDefault" ml={1}>{value || "-"}</FieldValue>
      {value && (
        <CopyButton size="small" onClick={() => handleCopyToClipboard(value, fieldName)}>
          <CopyIcon />
        </CopyButton>
      )}
    </CopyableFieldContainer>
  );
};

export const LabeledField = ({ label, value, width = '20%' }) => {
  return (
    <FieldRow>
      <FieldLabel variant="bodyLargeBold" sx={{ width }}>{label}</FieldLabel>
      <FieldValue variant="bodyLargeDefault" ml={1}>{value || "-"}</FieldValue>
    </FieldRow>
  );
};

export const LabeledCopyableField = ({ label, value, fieldName, handleCopyToClipboard, width = '20%' }) => {
  return (
    <FieldRow>
      <FieldLabel variant="bodyLargeBold" sx={{ width }}>{label}</FieldLabel>
      <CopyableField value={value} fieldName={fieldName} handleCopyToClipboard={handleCopyToClipboard} />
    </FieldRow>
  );
};