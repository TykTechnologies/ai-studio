import React, { useEffect, useState, useCallback } from "react";
import apiClient from "../utils/apiClient";
import {
  Typography,
  Box,
  Tabs,
  Tab,
  Alert,
  CircularProgress,
  Button,
  Chip,
} from "@mui/material";
import { TitleBox, StyledPaper } from "../styles/sharedStyles";
import DateRangePicker from "../components/common/DateRangePicker";
import DownloadIcon from "@mui/icons-material/Download";
import SecurityIcon from "@mui/icons-material/Security";

import SummaryCards from "../components/compliance/SummaryCards";
import HighRiskAppsTable from "../components/compliance/HighRiskAppsTable";
import PolicyViolationsTab from "../components/compliance/PolicyViolationsTab";
import BudgetComplianceTab from "../components/compliance/BudgetComplianceTab";
import ErrorsTab from "../components/compliance/ErrorsTab";
import AppRiskModal from "../components/compliance/AppRiskModal";

const ComplianceOverview = () => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [isEnterpriseAvailable, setIsEnterpriseAvailable] = useState(false);

  // Date range state
  const [startDate, setStartDate] = useState(
    new Date(new Date().getTime() - 7 * 24 * 60 * 60 * 1000)
      .toISOString()
      .split("T")[0]
  );
  const [endDate, setEndDate] = useState(
    new Date().toISOString().split("T")[0]
  );

  // Tab state
  const [activeTab, setActiveTab] = useState(0);

  // Data states
  const [summary, setSummary] = useState(null);
  const [highRiskApps, setHighRiskApps] = useState([]);
  const [policyViolations, setPolicyViolations] = useState(null);
  const [budgetAlerts, setBudgetAlerts] = useState(null);
  const [errors, setErrors] = useState(null);

  // Modal state
  const [selectedAppId, setSelectedAppId] = useState(null);
  const [modalOpen, setModalOpen] = useState(false);

  // Check if enterprise features are available
  useEffect(() => {
    const checkEnterprise = async () => {
      try {
        const response = await apiClient.get("/compliance/available");
        setIsEnterpriseAvailable(response.data.available);
        if (response.data.available) {
          fetchData();
        } else {
          setLoading(false);
        }
      } catch (err) {
        setError("Failed to check enterprise availability");
        setLoading(false);
      }
    };
    checkEnterprise();
  }, []);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const params = {
        start_date: startDate,
        end_date: endDate,
      };

      const [summaryRes, highRiskRes, policyRes, budgetRes, errorsRes] =
        await Promise.all([
          apiClient.get("/compliance/summary", { params }),
          apiClient.get("/compliance/high-risk-apps", { params: { ...params, limit: 10 } }),
          apiClient.get("/compliance/policy-violations", { params }),
          apiClient.get("/compliance/budget-alerts", { params }),
          apiClient.get("/compliance/errors", { params }),
        ]);

      setSummary(summaryRes.data);
      setHighRiskApps(highRiskRes.data || []);
      setPolicyViolations(policyRes.data);
      setBudgetAlerts(budgetRes.data);
      setErrors(errorsRes.data);
    } catch (err) {
      if (err.response?.status === 403) {
        setIsEnterpriseAvailable(false);
      } else {
        setError("Failed to fetch compliance data");
      }
    } finally {
      setLoading(false);
    }
  }, [startDate, endDate]);

  const handleDateChange = () => {
    fetchData();
  };

  const handleTabChange = (event, newValue) => {
    setActiveTab(newValue);
  };

  const handleAppClick = (appId) => {
    setSelectedAppId(appId);
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setSelectedAppId(null);
  };

  const handleExport = async (view) => {
    try {
      const response = await apiClient.get("/compliance/export", {
        params: {
          start_date: startDate,
          end_date: endDate,
          view: view,
        },
        responseType: "blob",
      });

      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement("a");
      link.href = url;
      link.setAttribute("download", `compliance_${view}_${startDate}_${endDate}.csv`);
      document.body.appendChild(link);
      link.click();
      link.remove();
    } catch (err) {
      console.error("Export failed:", err);
    }
  };

  const getExportView = () => {
    const views = ["policy", "budget", "errors"];
    return views[activeTab] || "policy";
  };

  if (!isEnterpriseAvailable && !loading) {
    return (
      <Box sx={{ p: 3 }}>
        <TitleBox top="64px">
          <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
            <SecurityIcon sx={{ fontSize: 32 }} />
            <Typography variant="headingXLarge">Compliance Overview</Typography>
          </Box>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Alert severity="info" sx={{ mb: 3 }}>
            <Typography variant="h6" gutterBottom>
              Enterprise Feature
            </Typography>
            <Typography>
              The Compliance Overview dashboard is an Enterprise Edition feature that provides:
            </Typography>
            <ul>
              <li>Real-time access anomaly detection</li>
              <li>Policy violation tracking</li>
              <li>Budget compliance monitoring</li>
              <li>Risk scoring for applications</li>
              <li>CSV export capabilities</li>
            </ul>
            <Typography sx={{ mt: 2 }}>
              Visit{" "}
              <a href="https://tyk.io/ai-studio/pricing" target="_blank" rel="noopener noreferrer">
                tyk.io/ai-studio/pricing
              </a>{" "}
              for more information.
            </Typography>
          </Alert>
        </Box>
      </Box>
    );
  }

  return (
    <Box>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <SecurityIcon sx={{ fontSize: 32 }} />
          <Typography variant="headingXLarge">Compliance Overview</Typography>
          <Chip label="Enterprise" color="primary" size="small" />
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={setStartDate}
            onEndDateChange={setEndDate}
            onUpdate={handleDateChange}
            updateMode="manual"
            label=""
          />
          <Button
            variant="outlined"
            startIcon={<DownloadIcon />}
            onClick={() => handleExport(getExportView())}
            disabled={loading}
          >
            Export CSV
          </Button>
        </Box>
      </TitleBox>

      {loading ? (
        <Box sx={{ display: "flex", justifyContent: "center", p: 5 }}>
          <CircularProgress />
        </Box>
      ) : error ? (
        <Box sx={{ p: 3 }}>
          <Alert severity="error">{error}</Alert>
        </Box>
      ) : (
        <Box sx={{ p: 3 }}>
          {/* Summary Cards */}
          <SummaryCards summary={summary} />

          {/* High Risk Apps */}
          <StyledPaper sx={{ mt: 3, p: 3 }}>
            <Typography variant="h6" gutterBottom>
              High Risk Applications
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Applications with compliance issues that require attention
            </Typography>
            <HighRiskAppsTable
              apps={highRiskApps}
              onAppClick={handleAppClick}
            />
          </StyledPaper>

          {/* Tabbed Detail Views */}
          <StyledPaper sx={{ mt: 3 }}>
            <Tabs
              value={activeTab}
              onChange={handleTabChange}
              sx={{ borderBottom: 1, borderColor: "divider" }}
            >
              <Tab label="Policy Violations" />
              <Tab label="Budget Compliance" />
              <Tab label="Errors" />
            </Tabs>

            <Box sx={{ p: 3 }}>
              {activeTab === 0 && <PolicyViolationsTab data={policyViolations} onAppClick={handleAppClick} startDate={startDate} endDate={endDate} />}
              {activeTab === 1 && <BudgetComplianceTab data={budgetAlerts} />}
              {activeTab === 2 && <ErrorsTab data={errors} />}
            </Box>
          </StyledPaper>
        </Box>
      )}

      {/* App Risk Modal */}
      <AppRiskModal
        open={modalOpen}
        onClose={handleModalClose}
        appId={selectedAppId}
        startDate={startDate}
        endDate={endDate}
      />
    </Box>
  );
};

export default ComplianceOverview;
