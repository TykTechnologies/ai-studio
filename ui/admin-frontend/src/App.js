// src/App.js
import React from "react";
import { ThemeProvider } from "@mui/material/styles";
import theme from "./theme";
import {
  BrowserRouter as Router,
  Routes,
  Route,
  Navigate,
} from "react-router-dom";
import CssBaseline from "@mui/material/CssBaseline";
import Box from "@mui/material/Box";
import Toolbar from "@mui/material/Toolbar";

import MyAppBar from "./components/layout/AppBar";
import MyDrawer from "./components/layout/Drawer";
import Dashboard from "./pages/Dashboard";
import Login from "./pages/Login";

import Users from "./pages/Users";
import UserDetails from "./components/users/UserDetails";
import UserForm from "./components/users/UserForm";

import Groups from "./pages/Groups";
import GroupDetail from "./components/groups/GroupDetail";
import GroupForm from "./components/groups/GroupForm";

import LLMList from "./pages/LLMList";
import LLMDetails from "./components/llms/LLMDetails";
import LLMForm from "./components/llms/LLMForm";

import LLMSettingsList from "./pages/LLMSettingsList";
import LLMSettingsDetails from "./components/llm-settings/LLMSettingsDetails";
import LLMSettingsForm from "./components/llm-settings/LLMSettingsForm";

import ModelPriceList from "./pages/ModelPriceList";
import ModelPriceDetail from "./components/model-prices/ModelPriceDetail";
import ModelPriceForm from "./components/model-prices/ModelPriceForm";

const drawerWidth = 240;

// Dev mode flag
const isDevMode = true; // Set to true for dev mode, false otherwise

const PrivateRoute = ({ element }) => {
  const token = localStorage.getItem("token");

  if (isDevMode) {
    // In dev mode, allow access without token
    return element;
  }

  return token ? element : <Navigate to="/login" replace />;
};

function App() {
  return (
    <ThemeProvider theme={theme}>
      <Router>
        <Box sx={{ display: "flex" }}>
          <CssBaseline />
          <MyAppBar />
          <MyDrawer />
          <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
            <Toolbar />
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route
                path="/"
                element={<PrivateRoute element={<Dashboard />} />}
              />
              <Route
                path="/users"
                element={<PrivateRoute element={<Users />} />}
              />
              <Route path="/users/:id" element={<UserDetails />} />
              <Route
                path="/users/edit/:id"
                element={<PrivateRoute element={<UserForm />} />}
              />
              <Route
                path="/users/new"
                element={<PrivateRoute element={<UserForm />} />}
              />

              <Route
                path="/groups"
                element={<PrivateRoute element={<Groups />} />}
              />
              <Route
                path="/groups/:id"
                element={<PrivateRoute element={<GroupDetail />} />}
              />
              <Route
                path="/groups/edit/:id"
                element={<PrivateRoute element={<GroupForm />} />}
              />
              <Route
                path="/groups/new"
                element={<PrivateRoute element={<GroupForm />} />}
              />

              <Route
                path="/llms"
                element={<PrivateRoute element={<LLMList />} />}
              />
              <Route
                path="/llms/:id"
                element={<PrivateRoute element={<LLMDetails />} />}
              />
              <Route
                path="/llms/edit/:id"
                element={<PrivateRoute element={<LLMForm />} />}
              />
              <Route
                path="/llms/new"
                element={<PrivateRoute element={<LLMForm />} />}
              />

              <Route
                path="/llm-settings"
                element={<PrivateRoute element={<LLMSettingsList />} />}
              />
              <Route
                path="/llm-settings/:id"
                element={<PrivateRoute element={<LLMSettingsDetails />} />}
              />
              <Route
                path="/llm-settings/edit/:id"
                element={<PrivateRoute element={<LLMSettingsForm />} />}
              />
              <Route
                path="/llm-settings/new"
                element={<PrivateRoute element={<LLMSettingsForm />} />}
              />

              <Route
                path="/model-prices"
                element={<PrivateRoute element={<ModelPriceList />} />}
              />
              <Route
                path="/model-prices/:id"
                element={<PrivateRoute element={<ModelPriceDetail />} />}
              />
              <Route
                path="/model-prices/edit/:id"
                element={<PrivateRoute element={<ModelPriceForm />} />}
              />
              <Route
                path="/model-prices/new"
                element={<PrivateRoute element={<ModelPriceForm />} />}
              />

              {/* Add more routes as needed */}
            </Routes>
          </Box>
        </Box>
      </Router>
    </ThemeProvider>
  );
}

export default App;
