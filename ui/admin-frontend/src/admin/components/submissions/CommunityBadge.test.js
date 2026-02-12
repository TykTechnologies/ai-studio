import React from "react";
import { render, screen } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material";
import CommunityBadge from "./CommunityBadge";

const theme = createTheme();
const renderWithTheme = (ui) =>
  render(<ThemeProvider theme={theme}>{ui}</ThemeProvider>);

describe("CommunityBadge", () => {
  it("renders when show is true", () => {
    renderWithTheme(<CommunityBadge show={true} />);
    expect(screen.getByText("Community")).toBeInTheDocument();
  });

  it("renders by default (show defaults to true)", () => {
    renderWithTheme(<CommunityBadge />);
    expect(screen.getByText("Community")).toBeInTheDocument();
  });

  it("does not render when show is false", () => {
    const { container } = renderWithTheme(<CommunityBadge show={false} />);
    expect(container.firstChild).toBeNull();
  });
});
