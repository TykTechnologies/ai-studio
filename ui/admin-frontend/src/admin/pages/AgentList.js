import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Table,
  TableBody,
  TableHead,
  TableRow,
  Typography,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  Snackbar,
  Box,
  Chip,
  FormControl,
  InputLabel,
  Select,
  MenuItem as SelectMenuItem,
} from '@mui/material';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import AddIcon from '@mui/icons-material/Add';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import EmptyStateWidget from '../components/common/EmptyStateWidget';
import ConfirmationDialog from '../components/common/ConfirmationDialog';
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
} from '../styles/sharedStyles';
import PaginationControls from '../components/common/PaginationControls';
import usePagination from '../hooks/usePagination';
import agentService from '../services/agentService';

const AgentList = () => {
  const navigate = useNavigate();
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedAgent, setSelectedAgent] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success',
  });
  const [confirmDialog, setConfirmDialog] = useState({
    open: false,
    agentId: null,
    agentName: '',
    action: '',
  });
  const [statusFilter, setStatusFilter] = useState('all'); // 'all', 'active', 'inactive'

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchAgents = useCallback(async () => {
    try {
      setLoading(true);
      const isActive = statusFilter === 'all' ? undefined : statusFilter === 'active';

      const result = await agentService.listAgents(
        page,
        pageSize,
        '', // namespace
        isActive
      );

      setAgents(result.data || []);
      updatePaginationData(result.meta?.total || 0);
    } catch (err) {
      console.error('Error fetching agents:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, statusFilter, updatePaginationData]);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  const handleMenuOpen = (event, agent) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedAgent(agent);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleView = (id) => {
    navigate(`/admin/agents/${id}`);
    handleMenuClose();
  };

  const handleEdit = (id) => {
    navigate(`/admin/agents/edit/${id}`);
    handleMenuClose();
  };

  const handleCreate = () => {
    navigate('/admin/agents/new');
  };

  const handleRowClick = (id) => {
    navigate(`/admin/agents/${id}`);
  };

  const openConfirmDialog = (action, agent) => {
    setConfirmDialog({
      open: true,
      agentId: agent.id,
      agentName: agent.name,
      action,
    });
    handleMenuClose();
  };

  const closeConfirmDialog = () => {
    setConfirmDialog({
      open: false,
      agentId: null,
      agentName: '',
      action: '',
    });
  };

  const handleConfirmAction = async () => {
    const { agentId, action } = confirmDialog;

    try {
      switch (action) {
        case 'delete':
          await agentService.deleteAgent(agentId);
          setSnackbar({
            open: true,
            message: 'Agent deleted successfully',
            severity: 'success',
          });
          break;
        case 'activate':
          await agentService.activateAgent(agentId);
          setSnackbar({
            open: true,
            message: 'Agent activated successfully',
            severity: 'success',
          });
          break;
        case 'deactivate':
          await agentService.deactivateAgent(agentId);
          setSnackbar({
            open: true,
            message: 'Agent deactivated successfully',
            severity: 'success',
          });
          break;
        default:
          break;
      }
      fetchAgents();
    } catch (err) {
      setSnackbar({
        open: true,
        message: err.message,
        severity: 'error',
      });
    } finally {
      closeConfirmDialog();
    }
  };

  const handleSnackbarClose = () => {
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading && agents.length === 0) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <>
      <TitleBox>
        <Typography variant="headingXLarge">Agents</Typography>
        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <InputLabel>Status</InputLabel>
            <Select
              value={statusFilter}
              label="Status"
              onChange={(e) => setStatusFilter(e.target.value)}
            >
              <SelectMenuItem value="all">All</SelectMenuItem>
              <SelectMenuItem value="active">Active</SelectMenuItem>
              <SelectMenuItem value="inactive">Inactive</SelectMenuItem>
            </Select>
          </FormControl>
          <PrimaryButton startIcon={<AddIcon />} onClick={handleCreate}>
            Create Agent
          </PrimaryButton>
        </Box>
      </TitleBox>

      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {agents.length === 0 && !loading ? (
          <EmptyStateWidget
            icon={SmartToyIcon}
            title="No agents found"
            description="Create your first agent to enable agentic workflows"
            actionLabel="Create Agent"
            onAction={handleCreate}
          />
        ) : (
          <StyledPaper>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Plugin</StyledTableHeaderCell>
                  <StyledTableHeaderCell>App</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Groups</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {agents.map((agent) => (
                  <StyledTableRow
                    key={agent.id}
                    hover
                    onClick={() => handleRowClick(agent.id)}
                    sx={{ cursor: 'pointer' }}
                  >
                    <StyledTableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <SmartToyIcon fontSize="small" color="primary" />
                        <Typography variant="bodyMedium">{agent.name}</Typography>
                      </Box>
                    </StyledTableCell>
                    <StyledTableCell>
                      <Typography
                        variant="bodySmallDefault"
                        sx={{
                          maxWidth: 200,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {agent.description || '-'}
                      </Typography>
                    </StyledTableCell>
                    <StyledTableCell>
                      {agent.plugin?.name || 'Unknown'}
                    </StyledTableCell>
                    <StyledTableCell>
                      {agent.app?.name || 'Unknown'}
                    </StyledTableCell>
                    <StyledTableCell>
                      {agent.groups.length === 0 ? (
                        <Chip label="Public" size="small" />
                      ) : (
                        <Typography variant="bodySmallDefault">
                          {agent.groups.length} group{agent.groups.length !== 1 ? 's' : ''}
                        </Typography>
                      )}
                    </StyledTableCell>
                    <StyledTableCell>
                      <Chip
                        label={agent.isActive ? 'Active' : 'Inactive'}
                        color={agent.isActive ? 'success' : 'default'}
                        size="small"
                      />
                    </StyledTableCell>
                    <StyledTableCell align="right">
                      <IconButton
                        onClick={(e) => handleMenuOpen(e, agent)}
                        size="small"
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </StyledTableCell>
                  </StyledTableRow>
                ))}
              </TableBody>
            </Table>

            <PaginationControls
              page={page}
              pageSize={pageSize}
              totalPages={totalPages}
              onPageChange={handlePageChange}
              onPageSizeChange={handlePageSizeChange}
            />
          </StyledPaper>
        )}
      </ContentBox>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={() => handleView(selectedAgent?.id)}>View Details</MenuItem>
        <MenuItem onClick={() => handleEdit(selectedAgent?.id)}>Edit</MenuItem>
        {selectedAgent?.isActive ? (
          <MenuItem onClick={() => openConfirmDialog('deactivate', selectedAgent)}>
            Deactivate
          </MenuItem>
        ) : (
          <MenuItem onClick={() => openConfirmDialog('activate', selectedAgent)}>
            Activate
          </MenuItem>
        )}
        <MenuItem onClick={() => openConfirmDialog('delete', selectedAgent)}>
          Delete
        </MenuItem>
      </Menu>

      <ConfirmationDialog
        open={confirmDialog.open}
        title={`${confirmDialog.action === 'delete' ? 'Delete' : confirmDialog.action === 'activate' ? 'Activate' : 'Deactivate'} Agent`}
        message={`Are you sure you want to ${confirmDialog.action} the agent "${confirmDialog.agentName}"?`}
        onConfirm={handleConfirmAction}
        onCancel={closeConfirmDialog}
      />

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleSnackbarClose}
      >
        <Alert
          onClose={handleSnackbarClose}
          severity={snackbar.severity}
          sx={{ width: '100%' }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default AgentList;
