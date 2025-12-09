import React, { useState, useEffect } from 'react';
import {
  Box,
  Container,
  Typography,
  TextField,
  MenuItem,
  Grid,
  Pagination,
  CircularProgress,
  Alert,
  Button,
  FormControl,
  InputLabel,
  Select,
  FormControlLabel,
  Switch,
  Paper,
  Chip,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  Search as SearchIcon,
} from '@mui/icons-material';
import marketplaceService from '../../services/marketplaceService';
import PluginCard from './PluginCard';
import PluginDetailModal from './PluginDetailModal';
import { useNavigate } from 'react-router-dom';
import useAdminData from '../../hooks/useAdminData';

const Marketplace = () => {
  const navigate = useNavigate();
  const { config } = useAdminData();
  const [plugins, setPlugins] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedPlugin, setSelectedPlugin] = useState(null);
  const [detailModalOpen, setDetailModalOpen] = useState(false);

  // Pagination
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [total, setTotal] = useState(0);
  const pageSize = 12;

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [category, setCategory] = useState('all');
  const [publisher, setPublisher] = useState('all');
  const [maturity, setMaturity] = useState('all');
  const [includeDeprecated, setIncludeDeprecated] = useState(false);

  // Filter options
  const [categories, setCategories] = useState([]);
  const [publishers, setPublishers] = useState([]);

  // Sync status
  const [syncing, setSyncing] = useState(false);

  useEffect(() => {
    loadFilterOptions();
  }, []);

  useEffect(() => {
    loadPlugins();
  }, [page, category, publisher, maturity, includeDeprecated]);

  const loadFilterOptions = async () => {
    try {
      const [categoriesData, publishersData] = await Promise.all([
        marketplaceService.getCategories(),
        marketplaceService.getPublishers(),
      ]);
      setCategories(categoriesData);
      setPublishers(publishersData);
    } catch (err) {
      console.error('Failed to load filter options:', err);
    }
  };

  const loadPlugins = async () => {
    setLoading(true);
    setError(null);

    try {
      const data = await marketplaceService.listPlugins({
        page,
        page_size: pageSize,
        category: category !== 'all' ? category : undefined,
        publisher: publisher !== 'all' ? publisher : undefined,
        maturity: maturity !== 'all' ? maturity : undefined,
        search: searchQuery || undefined,
        include_deprecated: includeDeprecated,
      });

      setPlugins(data.plugins || []);
      setTotal(data.total || 0);
      setTotalPages(data.total_pages || 0);
    } catch (err) {
      setError(err.message || 'Failed to load marketplace plugins');
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = () => {
    setPage(1); // Reset to first page
    loadPlugins();
  };

  const handleSearchKeyPress = (e) => {
    if (e.key === 'Enter') {
      handleSearch();
    }
  };

  const handleViewDetails = async (plugin) => {
    setSelectedPlugin(plugin);
    setDetailModalOpen(true);
  };

  const handleInstall = async (plugin) => {
    try {
      // Get install metadata
      const metadata = await marketplaceService.getInstallMetadata(
        plugin.plugin_id,
        plugin.version
      );

      // Navigate to plugin creation wizard with pre-filled data
      navigate('/admin/plugins/create', {
        state: {
          fromMarketplace: true,
          marketplaceData: metadata,
        },
      });
    } catch (err) {
      setError(`Failed to prepare installation: ${err.message}`);
    }
  };

  const handleSync = async () => {
    setSyncing(true);
    try {
      await marketplaceService.syncMarketplace();
      // Wait a moment then reload
      setTimeout(() => {
        loadPlugins();
        setSyncing(false);
      }, 2000);
    } catch (err) {
      setError(`Failed to sync marketplace: ${err.message}`);
      setSyncing(false);
    }
  };

  const handlePageChange = (event, value) => {
    setPage(value);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  return (
    <Container maxWidth="xl" sx={{ mt: 4, mb: 4 }}>
      <Box sx={{ mb: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="h4" component="h1">
            Plugin Marketplace
          </Typography>
          <Button
            variant="outlined"
            startIcon={syncing ? <CircularProgress size={16} /> : <RefreshIcon />}
            onClick={handleSync}
            disabled={syncing}
          >
            {syncing ? 'Syncing...' : 'Sync Marketplace'}
          </Button>
        </Box>
        <Typography variant="body1" color="text.secondary">
          {config?.is_enterprise
            ? 'Browse and install plugins from configured marketplace sources'
            : 'Browse and install plugins from the Tyk AI Studio marketplace'}
        </Typography>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      <Paper sx={{ p: 3, mb: 3 }}>
        <Grid container spacing={2} alignItems="center">
          <Grid item xs={12} md={4}>
            <TextField
              fullWidth
              label="Search plugins"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyPress={handleSearchKeyPress}
              InputProps={{
                endAdornment: (
                  <Button size="small" onClick={handleSearch}>
                    <SearchIcon />
                  </Button>
                ),
              }}
            />
          </Grid>
          <Grid item xs={12} sm={6} md={2}>
            <FormControl fullWidth>
              <InputLabel>Category</InputLabel>
              <Select
                value={category}
                label="Category"
                onChange={(e) => {
                  setCategory(e.target.value);
                  setPage(1);
                }}
              >
                <MenuItem value="all">All Categories</MenuItem>
                {categories.map((cat) => (
                  <MenuItem key={cat} value={cat}>
                    {cat}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} sm={6} md={2}>
            <FormControl fullWidth>
              <InputLabel>Publisher</InputLabel>
              <Select
                value={publisher}
                label="Publisher"
                onChange={(e) => {
                  setPublisher(e.target.value);
                  setPage(1);
                }}
              >
                <MenuItem value="all">All Publishers</MenuItem>
                {publishers.map((pub) => (
                  <MenuItem key={pub} value={pub}>
                    {pub}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} sm={6} md={2}>
            <FormControl fullWidth>
              <InputLabel>Maturity</InputLabel>
              <Select
                value={maturity}
                label="Maturity"
                onChange={(e) => {
                  setMaturity(e.target.value);
                  setPage(1);
                }}
              >
                <MenuItem value="all">All</MenuItem>
                <MenuItem value="stable">Stable</MenuItem>
                <MenuItem value="beta">Beta</MenuItem>
                <MenuItem value="alpha">Alpha</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} sm={6} md={2}>
            <FormControlLabel
              control={
                <Switch
                  checked={includeDeprecated}
                  onChange={(e) => {
                    setIncludeDeprecated(e.target.checked);
                    setPage(1);
                  }}
                />
              }
              label="Show deprecated"
            />
          </Grid>
        </Grid>
      </Paper>

      {total > 0 && (
        <Box sx={{ mb: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="body2" color="text.secondary">
            Showing {plugins.length} of {total} plugins
          </Typography>
        </Box>
      )}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <CircularProgress />
        </Box>
      ) : plugins.length === 0 ? (
        <Paper sx={{ p: 8, textAlign: 'center' }}>
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No plugins found
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Try adjusting your search filters
          </Typography>
        </Paper>
      ) : (
        <>
          <Grid container spacing={3}>
            {plugins.map((plugin) => (
              <Grid item xs={12} sm={6} md={4} lg={3} key={`${plugin.plugin_id}-${plugin.version}`}>
                <PluginCard
                  plugin={plugin}
                  onViewDetails={handleViewDetails}
                  onInstall={handleInstall}
                />
              </Grid>
            ))}
          </Grid>

          {totalPages > 1 && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
              <Pagination
                count={totalPages}
                page={page}
                onChange={handlePageChange}
                color="primary"
                size="large"
              />
            </Box>
          )}
        </>
      )}

      {selectedPlugin && (
        <PluginDetailModal
          open={detailModalOpen}
          plugin={selectedPlugin}
          onClose={() => {
            setDetailModalOpen(false);
            setSelectedPlugin(null);
          }}
          onInstall={handleInstall}
        />
      )}
    </Container>
  );
};

export default Marketplace;
