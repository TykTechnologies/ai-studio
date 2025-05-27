import React from "react";
import { render } from "@testing-library/react";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "./test-theme";

export const renderWithTheme = (ui, options) => {
  return render(
    <ThemeProvider theme={testTheme}>
      {ui}
    </ThemeProvider>,
    options
  );
};