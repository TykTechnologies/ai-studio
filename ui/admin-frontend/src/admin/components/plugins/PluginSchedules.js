import React, { useState, useEffect } from 'react';
import {
  Box,
  Paper,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Chip,
  IconButton,
  Tooltip,
  CircularProgress,
  Alert,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Switch,
  FormControlLabel,
  Collapse,
  List,
  ListItem,
  ListItemText,
} from '@mui/material';
import {
  Schedule as ScheduleIcon,
  History as HistoryIcon,
  Delete as DeleteIcon,
  CheckCircle as SuccessIcon,
  Error as ErrorIcon,
  AccessTime as TimeoutIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import scheduleService from '../../services/scheduleService';

const PluginSchedules = ({ pluginId, pluginIsActive }) => {
  const [schedules, setSchedules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedSchedule, setSelectedSchedule] = useState(null);
  const [executionDialogOpen, setExecutionDialogOpen] = useState(false);
  const [executions, setExecutions] = useState([]);
  const [executionsLoading, setExecutionsLoading] = useState(false);
  const [expandedSchedule, setExpandedSchedule] = useState(null);

  useEffect(() => {
    fetchSchedules();
  }, [pluginId]);

  const fetchSchedules = async () => {
    setLoading(true);
    setError(null);

    try {
      const result = await scheduleService.getPluginSchedules(pluginId);
      setSchedules(result.schedules || []);
    } catch (err) {
      console.error('Error fetching schedules:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleViewExecutions = async (schedule) => {
    setSelectedSchedule(schedule);
    setExecutionDialogOpen(true);
    setExecutionsLoading(true);

    try {
      const result = await scheduleService.getScheduleExecutions(pluginId, schedule.id);
      setExecutions(result.executions || []);
    } catch (err) {
      console.error('Error fetching executions:', err);
      setError(err.message);
    } finally {
      setExecutionsLoading(false);
    }
  };

  const handleToggleEnabled = async (schedule) => {
    try {
      await scheduleService.updateSchedule(pluginId, schedule.id, {
        enabled: !schedule.enabled,
      });
      fetchSchedules(); // Refresh list
    } catch (err) {
      console.error('Error updating schedule:', err);
      setError(err.message);
    }
  };

  const handleDeleteSchedule = async (schedule) => {
    if (window.confirm(`Are you sure you want to delete the schedule "${schedule.name}"?`)) {
      try {
        await scheduleService.deleteSchedule(pluginId, schedule.id);
        fetchSchedules(); // Refresh list
      } catch (err) {
        console.error('Error deleting schedule:', err);
        setError(err.message);
      }
    }
  };

  const formatDuration = (ms) => {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatDateTime = (dateStr) => {
    if (!dateStr) return 'Never';
    const date = new Date(dateStr);
    return date.toLocaleString();
  };

  const getStatusIcon = (status) => {
    switch (status) {
      case 'completed':
        return <SuccessIcon color="success" fontSize="small" />;
      case 'failed':
        return <ErrorIcon color="error" fontSize="small" />;
      case 'timeout':
        return <TimeoutIcon color="warning" fontSize="small" />;
      default:
        return <CircularProgress size={16} />;
    }
  };

  const getStatusColor = (status) => {
    switch (status) {
      case 'completed':
        return 'success';
      case 'failed':
        return 'error';
      case 'timeout':
        return 'warning';
      case 'running':
        return 'info';
      default:
        return 'default';
    }
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" p={4}>
        <CircularProgress />
      </Box>
    );
  }

  if (schedules.length === 0) {
    return (
      <Paper sx={{ p: 3, textAlign: 'center' }}>
        <ScheduleIcon sx={{ fontSize: 60, color: 'text.secondary', mb: 2 }} />
        <Typography variant="h6" color="text.secondary" gutterBottom>
          No Scheduled Tasks
        </Typography>
        <Typography variant="body2" color="text.secondary">
          This plugin does not have any scheduled tasks configured.
        </Typography>
      </Paper>
    );
  }

  return (
    <Box>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {!pluginIsActive && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          Plugin is not active. Scheduled tasks will not run until the plugin is activated.
        </Alert>
      )}

      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">Scheduled Tasks</Typography>
        <Button
          startIcon={<RefreshIcon />}
          onClick={fetchSchedules}
          size="small"
        >
          Refresh
        </Button>
      </Box>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Schedule</TableCell>
              <TableCell>Timezone</TableCell>
              <TableCell>Timeout</TableCell>
              <TableCell>Last Run</TableCell>
              <TableCell>Next Run</TableCell>
              <TableCell>Status</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {schedules.map((schedule) => (
              <React.Fragment key={schedule.id}>
                <TableRow hover>
                  <TableCell>
                    <Box display="flex" alignItems="center" gap={1}>
                      <ScheduleIcon fontSize="small" color="action" />
                      <Box>
                        <Typography variant="body2" fontWeight="medium">
                          {schedule.name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          ID: {schedule.schedule_id}
                        </Typography>
                      </Box>
                    </Box>
                  </TableCell>
                  <TableCell>
                    <code style={{ fontSize: '0.85rem' }}>{schedule.cron_expr}</code>
                  </TableCell>
                  <TableCell>{schedule.timezone}</TableCell>
                  <TableCell>{schedule.timeout_seconds}s</TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {formatDateTime(schedule.last_run)}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {formatDateTime(schedule.next_run)}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={schedule.enabled}
                          onChange={() => handleToggleEnabled(schedule)}
                          size="small"
                        />
                      }
                      label={schedule.enabled ? 'Enabled' : 'Disabled'}
                    />
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title="View execution history">
                      <IconButton
                        size="small"
                        onClick={() => handleViewExecutions(schedule)}
                      >
                        <HistoryIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Expand details">
                      <IconButton
                        size="small"
                        onClick={() => setExpandedSchedule(
                          expandedSchedule === schedule.id ? null : schedule.id
                        )}
                      >
                        {expandedSchedule === schedule.id ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete schedule">
                      <IconButton
                        size="small"
                        color="error"
                        onClick={() => handleDeleteSchedule(schedule)}
                      >
                        <DeleteIcon />
                      </IconButton>
                    </Tooltip>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell colSpan={8} sx={{ p: 0 }}>
                    <Collapse in={expandedSchedule === schedule.id} timeout="auto" unmountOnExit>
                      <Box sx={{ p: 2, bgcolor: 'background.default' }}>
                        <Typography variant="subtitle2" gutterBottom>
                          Configuration
                        </Typography>
                        <pre style={{
                          background: '#f5f5f5',
                          padding: '8px',
                          borderRadius: '4px',
                          fontSize: '0.85rem',
                          overflow: 'auto',
                        }}>
                          {schedule.config || '{}'}
                        </pre>
                      </Box>
                    </Collapse>
                  </TableCell>
                </TableRow>
              </React.Fragment>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Execution History Dialog */}
      <Dialog
        open={executionDialogOpen}
        onClose={() => setExecutionDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          Execution History
          {selectedSchedule && (
            <Typography variant="caption" display="block" color="text.secondary">
              {selectedSchedule.name}
            </Typography>
          )}
        </DialogTitle>
        <DialogContent>
          {executionsLoading ? (
            <Box display="flex" justifyContent="center" p={4}>
              <CircularProgress />
            </Box>
          ) : executions.length === 0 ? (
            <Alert severity="info">
              No executions recorded yet. This schedule hasn't run or was just created.
            </Alert>
          ) : (
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Started</TableCell>
                    <TableCell>Status</TableCell>
                    <TableCell>Duration</TableCell>
                    <TableCell>Details</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {executions.map((execution) => (
                    <TableRow key={execution.id}>
                      <TableCell>
                        <Typography variant="body2">
                          {formatDateTime(execution.started_at)}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          icon={getStatusIcon(execution.status)}
                          label={execution.status}
                          size="small"
                          color={getStatusColor(execution.status)}
                        />
                      </TableCell>
                      <TableCell>
                        {execution.completed_at ? formatDuration(execution.duration) : 'N/A'}
                      </TableCell>
                      <TableCell>
                        {execution.error ? (
                          <Tooltip title={execution.error}>
                            <Typography variant="caption" color="error" sx={{ cursor: 'pointer' }}>
                              {execution.error.length > 50
                                ? `${execution.error.substring(0, 50)}...`
                                : execution.error}
                            </Typography>
                          </Tooltip>
                        ) : (
                          <Typography variant="caption" color="success.main">
                            Success
                          </Typography>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setExecutionDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default PluginSchedules;
