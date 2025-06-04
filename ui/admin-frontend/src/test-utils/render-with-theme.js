import React from "react";
import { render } from "@testing-library/react";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "./test-theme";

export const renderWithTheme = (ui, options) => {
  return render(
    <ThemeProvider theme={testTheme}>
      {ui}
    </ThemeProvider>,
    options
  );
};

export const renderWithRouterAndTheme = (ui, { route = '/', initialEntries = [route], ...options } = {}) => {
  return render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={initialEntries}>
        {ui}
      </MemoryRouter>
    </ThemeProvider>,
    options
  );
};

export const renderWithRoutesAndTheme = (ui, { routes = [], initialEntry = '/', ...options } = {}) => {
  return render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          {routes.map((route, index) => (
            <Route key={index} path={route.path} element={route.element} />
          ))}
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
    options
  );
};