import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import ActionModal from "./ActionModal";

// Create a more comprehensive test theme with all needed palette values
const theme = createTheme({
  palette: {
    text: {
      primary: '#000000',
      secondary: '#666666',
      defaultSubdued: '#454545',
      neutralDisabled: '#AAAAAA'
    },
    background: {
      paper: '#FFFFFF',
      default: '#F5F5F5',
      surfaceNeutralHover: '#F0F0F0',
      surfaceNeutralDisabled: '#EEEEEE',
      buttonPrimaryDefault: '#3F51B5',
      buttonPrimaryDefaultHover: '#303F9F',
      buttonPrimaryOutlineHover: '#E8EAF6',
      defaultSubdued: '#FAFAFA'
    },
    border: {
      neutralDefault: '#E0E0E0',
      neutralHovered: '#BDBDBD',
      criticalDefault: '#F44336',
      criticalHover: '#D32F2F'
    },
    primary: {
      main: '#3F51B5',
      light: '#7986CB'
    },
    custom: {
      white: '#FFFFFF',
      purpleExtraDark: '#1A237E'
    }
  }
});

// Test wrapper with theme provider
const renderWithTheme = (ui) => {
  return render(
    <ThemeProvider theme={theme}>
      {ui}
    </ThemeProvider>
  );
};

describe("ActionModal", () => {
  const mockOnClose = jest.fn();
  const mockOnPrimaryAction = jest.fn();
  const mockOnSecondaryAction = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders with default props when open", () => {
    renderWithTheme(
      <ActionModal open={true} title="Test Title" onClose={mockOnClose} onPrimaryAction={mockOnPrimaryAction}>
        <div data-testid="modal-content">Content</div>
      </ActionModal>
    );

    expect(screen.getByText("Test Title")).toBeInTheDocument();
    expect(screen.getByTestId("modal-content")).toBeInTheDocument();
    expect(screen.getByText("Save")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("does not render when open is false", () => {
    renderWithTheme(
      <ActionModal open={false} title="Test Title" onClose={mockOnClose} onPrimaryAction={mockOnPrimaryAction}>
        <div data-testid="modal-content">Content</div>
      </ActionModal>
    );

    expect(screen.queryByText("Test Title")).not.toBeInTheDocument();
  });

  it("renders with custom button labels", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Title"
        primaryButtonLabel="Custom Save"
        secondaryButtonLabel="Custom Cancel"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
      >
        <div>Content</div>
      </ActionModal>
    );

    expect(screen.getByText("Custom Save")).toBeInTheDocument();
    expect(screen.getByText("Custom Cancel")).toBeInTheDocument();
  });

  it("calls onPrimaryAction when primary button is clicked", () => {
    renderWithTheme(
      <ActionModal open={true} title="Test Title" onClose={mockOnClose} onPrimaryAction={mockOnPrimaryAction}>
        <div>Content</div>
      </ActionModal>
    );

    fireEvent.click(screen.getByText("Save"));
    expect(mockOnPrimaryAction).toHaveBeenCalledTimes(1);
  });

  it("calls onSecondaryAction when secondary button is clicked", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Title"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
        onSecondaryAction={mockOnSecondaryAction}
      >
        <div>Content</div>
      </ActionModal>
    );

    fireEvent.click(screen.getByText("Cancel"));
    expect(mockOnSecondaryAction).toHaveBeenCalledTimes(1);
    expect(mockOnClose).not.toHaveBeenCalled();
  });

  it("calls onClose when secondary button is clicked and onSecondaryAction is not provided", () => {
    renderWithTheme(
      <ActionModal open={true} title="Test Title" onClose={mockOnClose} onPrimaryAction={mockOnPrimaryAction}>
        <div>Content</div>
      </ActionModal>
    );

    fireEvent.click(screen.getByText("Cancel"));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when escape key is pressed", () => {
    renderWithTheme(
      <ActionModal open={true} title="Test Title" onClose={mockOnClose} onPrimaryAction={mockOnPrimaryAction}>
        <div>Content</div>
      </ActionModal>
    );

    // Get the dialog element and fire the event on it instead of document
    const dialog = screen.getByRole("dialog");
    fireEvent.keyDown(dialog, { key: "Escape", code: "Escape" });
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });
});