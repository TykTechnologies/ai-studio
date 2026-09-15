import React, { useMemo, useState } from 'react';
import { Box, Button, Chip, Paper, TextField, Typography } from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import HowToVoteIcon from '@mui/icons-material/HowToVote';
import { Form } from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';
import { ResultViewer } from './ToolCallCard';

const ArgsSummary = ({ args }) => {
  const entries = Object.entries(args || {});
  if (entries.length === 0) return null;
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5, my: 1 }}>
      {entries.map(([k, v]) => (
        <Typography key={k} variant="body2">
          <strong>{k}:</strong> {typeof v === 'string' ? v : JSON.stringify(v)}
        </Typography>
      ))}
    </Box>
  );
};

const Answered = ({ toolName, result, isError }) => (
  <Paper variant="outlined" sx={{ my: 1, p: 1.5, borderLeft: '4px solid', borderLeftColor: isError ? 'error.main' : 'success.main' }} data-testid="human-tool-answered">
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
      <CheckCircleIcon fontSize="small" color={isError ? 'error' : 'success'} />
      <Typography variant="bodyMedium" fontWeight={500}>{toolName}</Typography>
      <Chip label={isError ? 'declined' : 'answered'} size="small" variant="outlined" color={isError ? 'error' : 'success'} />
    </Box>
    <Box sx={{ mt: 1 }}>
      <ResultViewer result={result} />
    </Box>
  </Paper>
);

const ApprovalCard = ({ title, description, args, addResult }) => {
  const [comment, setComment] = useState('');
  return (
    <>
      <Typography variant="bodyMedium" fontWeight={500}>{title}</Typography>
      {description && <Typography variant="body2" color="text.secondary">{description}</Typography>}
      <ArgsSummary args={args} />
      <TextField
        size="small"
        fullWidth
        placeholder="Optional comment"
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        sx={{ my: 1 }}
      />
      <Box sx={{ display: 'flex', gap: 1 }}>
        <Button variant="contained" color="success" size="small" onClick={() => addResult({ approved: true, comment })}>
          Approve
        </Button>
        <Button variant="outlined" color="error" size="small" onClick={() => addResult({ approved: false, comment })}>
          Reject
        </Button>
      </Box>
    </>
  );
};

const FormCard = ({ title, description, args, schema, addResult }) => {
  const formSchema = useMemo(() => {
    if (schema && typeof schema === 'object' && Object.keys(schema).length > 0) return schema;
    return { type: 'object', properties: { response: { type: 'string', title: 'Your response' } }, required: ['response'] };
  }, [schema]);
  return (
    <>
      <Typography variant="bodyMedium" fontWeight={500}>{title}</Typography>
      {description && <Typography variant="body2" color="text.secondary">{description}</Typography>}
      <ArgsSummary args={args} />
      <Form
        schema={formSchema}
        validator={validator}
        onSubmit={({ formData }) => addResult(formData)}
        showErrorList={false}
      >
        <Box sx={{ display: 'flex', gap: 1, mt: 1 }}>
          <Button type="submit" variant="contained" size="small">Submit</Button>
          <Button variant="text" size="small" onClick={() => addResult({ error: 'The user declined to answer' }, true)}>
            Skip
          </Button>
        </Box>
      </Form>
    </>
  );
};

/**
 * Renderer for a client (human-in-the-loop) tool call: an approval or a
 * form, depending on the tool's UI kind. `addResult` hands the answer to the
 * runtime, which sends it back to the backend as tool_results.
 */
export const makeHumanToolRenderer = (info) => {
  const ui = info.ui || {};
  const kind = ui.kind || 'approval';
  const title = ui.title || info.description || info.name;
  const Renderer = ({ args, result, isError, status, addResult }) => {
    const answered = result !== undefined || status?.type === 'complete';
    if (answered) {
      return <Answered toolName={info.name} result={result} isError={isError} />;
    }
    const submit = (value, error = false) => {
      addResult(error ? { result: value, isError: true } : value);
    };
    return (
      <Paper variant="outlined" sx={{ my: 1, p: 2, borderLeft: '4px solid', borderLeftColor: 'warning.main', bgcolor: 'background.surfaceWarningDefault' }} data-testid="human-tool-card">
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
          <HowToVoteIcon fontSize="small" color="warning" />
          <Typography variant="caption" color="text.secondary">Your input is needed</Typography>
        </Box>
        {kind === 'form' ? (
          <FormCard title={title} description={ui.description} args={args} schema={ui.response_schema} addResult={submit} />
        ) : (
          <ApprovalCard title={title} description={ui.description} args={args} addResult={submit} />
        )}
      </Paper>
    );
  };
  Renderer.displayName = `HumanTool(${info.name})`;
  return Renderer;
};

export default makeHumanToolRenderer;
