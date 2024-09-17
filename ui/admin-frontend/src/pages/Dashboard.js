// src/Dashboard.js
import React, { useEffect, useState } from "react";
import apiClient from "../utils/apiClient";
import { Typography } from "@mui/material";

const Dashboard = () => {
  const [appsCount, setAppsCount] = useState(0);
  const [usersCount, setUsersCount] = useState(0);

  useEffect(() => {
    // Fetch some summary data from the API
    const fetchData = async () => {
      try {
        const appsResponse = await apiClient.get("/apps");
        const usersResponse = await apiClient.get("/users");
        setAppsCount(appsResponse.data.length);
        setUsersCount(usersResponse.data.length);
      } catch (error) {
        console.error("Error fetching dashboard data", error);
      }
    };

    fetchData();
  }, []);

  return (
    <div>
      <Typography variant="h4" gutterBottom>
        Welcome to the Admin Dashboard
      </Typography>
      <Typography variant="h6">Total Apps: {appsCount}</Typography>
      <Typography variant="h6">Total Users: {usersCount}</Typography>
    </div>
  );
};

export default Dashboard;
