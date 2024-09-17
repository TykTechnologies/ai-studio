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
import Users from "./pages/Users";
import UserDetails from "./components/users/UserDetails";
import Login from "./pages/Login";
import UserForm from "./components/users/UserForm";
import Apps from "./pages/Apps";
import AppForm from "./components/apps/AppForm";
import AppDetails from "./components/apps/AppDetails";
import Groups from "./pages/Groups";
import GroupForm from "./components/groups/GroupForm";
import Catalogues from "./pages/Catalogues";
import CatalogueForm from "./components/catalogues/CatalogueForm";
import Datasources from "./pages/Datasources";
import DatasourceForm from "./components/datasources/DatasourceForm";
import LLMs from "./pages/LLMs";
import LLMForm from "./components/llms/LLMForm";

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
                path="/apps"
                element={<PrivateRoute element={<Apps />} />}
              />
              <Route
                path="/apps/:id"
                element={<PrivateRoute element={<AppDetails />} />}
              />
              <Route
                path="/apps/edit/:id"
                element={<PrivateRoute element={<AppForm />} />}
              />
              <Route
                path="/apps/new"
                element={<PrivateRoute element={<AppForm />} />}
              />
              <Route
                path="/groups"
                element={<PrivateRoute element={<Groups />} />}
              />
              <Route
                path="/groups/new"
                element={<PrivateRoute element={<GroupForm />} />}
              />
              <Route
                path="/groups/edit/:id"
                element={<PrivateRoute element={<GroupForm />} />}
              />
              <Route
                path="/catalogues"
                element={<PrivateRoute element={<Catalogues />} />}
              />
              <Route
                path="/catalogues/new"
                element={<PrivateRoute element={<CatalogueForm />} />}
              />
              <Route
                path="/catalogues/edit/:id"
                element={<PrivateRoute element={<CatalogueForm />} />}
              />
              <Route
                path="/datasources"
                element={<PrivateRoute element={<Datasources />} />}
              />
              <Route
                path="/datasources/new"
                element={<PrivateRoute element={<DatasourceForm />} />}
              />
              <Route
                path="/datasources/edit/:id"
                element={<PrivateRoute element={<DatasourceForm />} />}
              />
              <Route
                path="/llms"
                element={<PrivateRoute element={<LLMs />} />}
              />
              <Route
                path="/llms/new"
                element={<PrivateRoute element={<LLMForm />} />}
              />
              <Route
                path="/llms/edit/:id"
                element={<PrivateRoute element={<LLMForm />} />}
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
