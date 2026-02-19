import React from "react";
import { render, screen } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material";
import StatusChip from "./StatusChip";

const theme = createTheme();
const renderWithTheme = (ui) =>
  render(<ThemeProvider theme={theme}>{ui}</ThemeProvider>);

describe("StatusChip", () => {
  it("renders draft status", () => {
    renderWithTheme(<StatusChip status="draft" />);
    expect(screen.getByText("Draft")).toBeInTheDocument();
  });

  it("renders submitted status as Pending Review", () => {
    renderWithTheme(<StatusChip status="submitted" />);
    expect(screen.getByText("Pending Review")).toBeInTheDocument();
  });

  it("renders in_review status", () => {
    renderWithTheme(<StatusChip status="in_review" />);
    expect(screen.getByText("In Review")).toBeInTheDocument();
  });

  it("renders approved status", () => {
    renderWithTheme(<StatusChip status="approved" />);
    expect(screen.getByText("Approved")).toBeInTheDocument();
  });

  it("renders rejected status", () => {
    renderWithTheme(<StatusChip status="rejected" />);
    expect(screen.getByText("Rejected")).toBeInTheDocument();
  });

  it("renders changes_requested status", () => {
    renderWithTheme(<StatusChip status="changes_requested" />);
    expect(screen.getByText("Changes Requested")).toBeInTheDocument();
  });

  it("handles unknown status gracefully", () => {
    renderWithTheme(<StatusChip status="some_unknown" />);
    expect(screen.getByText("some_unknown")).toBeInTheDocument();
  });

  it("handles null status gracefully", () => {
    renderWithTheme(<StatusChip status={null} />);
    expect(screen.getByText("Unknown")).toBeInTheDocument();
  });
});
