import React, { useState } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Typography,
  TextField,
  Box,
  Alert,
  CircularProgress,
} from "@mui/material";
import { PrimaryButton, SecondaryOutlineButton } from "../../styles/sharedStyles";
import { useEdition } from "../../context/EditionContext";
import EnterpriseFeatureBadge from "./EnterpriseFeatureBadge";
import apiClient from "../../utils/apiClient";

const ExportProxyLogsModal = ({
  open,
  onClose,
  sourceType, // 'app', 'llm', or 'user'
  sourceId,
  initialStartDate,
  initialEndDate,
  initialSearch = "",
}) => {
  const { isEnterprise } = useEdition();
  const [startDate, setStartDate] = useState(initialStartDate || "");
  const [endDate, setEndDate] = useState(initialEndDate || "");
  const [search, setSearch] = useState(initialSearch);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(false);

  // Reset state when modal opens
  React.useEffect(() => {
    if (open) {
      setStartDate(initialStartDate || "");
      setEndDate(initialEndDate || "");
      setSearch(initialSearch);
      setError(null);
      setSuccess(false);
    }
  }, [open, initialStartDate, initialEndDate, initialSearch]);

  const handleExport = async () => {
    if (!startDate || !endDate) {
      setError("Please select both start and end dates");
      return;
    }

    if (new Date(startDate) > new Date(endDate)) {
      setError("Start date must be before end date");
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await apiClient.post("/exports", {
        source_type: sourceType,
        source_id: sourceId,
        start_date: startDate,
        end_date: endDate,
        search: search || undefined,
      });

      setSuccess(true);
      // Close modal after a short delay
      setTimeout(() => {
        onClose();
        setSuccess(false);
      }, 3000);
    } catch (err) {
      if (err.response?.status === 402) {
        setError("This feature requires Enterprise Edition");
      } else {
        setError(
          err.response?.data?.errors?.[0]?.detail ||
            "Failed to start export. Please try again."
        );
      }
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    if (!loading) {
      onClose();
    }
  };

  // Get title and description based on source type
  const getTitle = () => {
    return sourceType === "user" ? "Export Chat History" : "Export Proxy Logs";
  };

  const getDescription = () => {
    switch (sourceType) {
      case "user":
        return "Export chat conversations for this user to a JSON file";
      case "app":
        return "Export logs for this application to a JSON file";
      case "llm":
        return "Export logs for this LLM vendor to a JSON file";
      default:
        return "Export logs to a JSON file";
    }
  };

  const getSearchPlaceholder = () => {
    return sourceType === "user"
      ? "Search conversation names..."
      : "Search request or response content";
  };

  const getFeatureName = () => {
    return sourceType === "user" ? "Chat History Export" : "Proxy Log Export";
  };

  const getFeatureDescription = () => {
    return sourceType === "user"
      ? "Export chat conversations to JSON files for analysis and compliance. This feature is available in the Enterprise Edition."
      : "Export proxy logs to JSON files for analysis and compliance. This feature is available in the Enterprise Edition.";
  };

  // Show enterprise badge if not enterprise
  if (!isEnterprise) {
    return (
      <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
        <DialogTitle>{getTitle()}</DialogTitle>
        <DialogContent>
          <EnterpriseFeatureBadge
            feature={getFeatureName()}
            description={getFeatureDescription()}
          />
        </DialogContent>
        <DialogActions>
          <SecondaryOutlineButton onClick={handleClose}>
            Close
          </SecondaryOutlineButton>
        </DialogActions>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        <Typography variant="h6">
          {getTitle()}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {getDescription()}
        </Typography>
      </DialogTitle>

      <DialogContent>
        {success ? (
          <Alert severity="success" sx={{ mt: 2 }}>
            Export started successfully! You will receive a notification when your export is ready for download.
          </Alert>
        ) : (
          <Box sx={{ mt: 2 }}>
            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}

            <Typography variant="subtitle2" gutterBottom>
              Date Range
            </Typography>
            <Box display="flex" gap={2} sx={{ mb: 3 }}>
              <TextField
                label="Start Date"
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                InputLabelProps={{ shrink: true }}
                size="small"
                fullWidth
                disabled={loading}
              />
              <TextField
                label="End Date"
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                InputLabelProps={{ shrink: true }}
                size="small"
                fullWidth
                disabled={loading}
              />
            </Box>

            <Typography variant="subtitle2" gutterBottom>
              Search Filter (Optional)
            </Typography>
            <TextField
              label={getSearchPlaceholder()}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              size="small"
              fullWidth
              disabled={loading}
              helperText={sourceType === "user"
                ? "Leave empty to export all conversations within the date range"
                : "Leave empty to export all logs within the date range"}
              sx={{ mb: 2 }}
            />

            <Alert severity="info" sx={{ mt: 2 }}>
              Large exports may take several minutes to process. You will receive a notification with a download link when the export is complete. The download link will expire after 24 hours.
            </Alert>
          </Box>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2 }}>
        <SecondaryOutlineButton onClick={handleClose} disabled={loading}>
          {success ? "Close" : "Cancel"}
        </SecondaryOutlineButton>
        {!success && (
          <PrimaryButton
            onClick={handleExport}
            disabled={loading || !startDate || !endDate}
          >
            {loading ? (
              <>
                <CircularProgress size={16} sx={{ mr: 1 }} color="inherit" />
                Starting Export...
              </>
            ) : (
              "Export"
            )}
          </PrimaryButton>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default ExportProxyLogsModal;
