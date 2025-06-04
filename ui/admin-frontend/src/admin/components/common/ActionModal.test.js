import React from "react";
import { screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import ActionModal from "./ActionModal";
import { renderWithTheme } from "../../../test-utils/render-with-theme";

jest.mock('@mui/material', () => require('../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('@mui/styled-engine', () => require('../../../test-utils/mui-mocks').muiStyledEngineMock);
jest.mock('@mui/material/styles', () => require('../../../test-utils/mui-mocks').muiStylesMock);
jest.mock('@mui/material/IconButton', () => require('../../../test-utils/mui-mocks').muiIconButtonMock);
jest.mock('../../styles/sharedStyles', () => require('../../../test-utils/styled-component-mocks').sharedStylesMock);
jest.mock('./styles', () => require('../../../test-utils/styled-component-mocks').actionModalStylesMock);

describe("ActionModal", () => {
  const mockOnClose = jest.fn();
  const mockOnPrimaryAction = jest.fn();
  const mockOnSecondaryAction = jest.fn();

  beforeEach(() => {
    mockOnClose.mockClear();
    mockOnPrimaryAction.mockClear();
    mockOnSecondaryAction.mockClear();
  });

  it("renders correctly with required props", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Modal"
        primaryButtonLabel="Save"
        secondaryButtonLabel="Cancel"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
      />
    );

    expect(screen.getByText("Test Modal")).toBeInTheDocument();
    expect(screen.getByText("Save")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("calls onPrimaryAction when primary button is clicked", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Modal"
        primaryButtonLabel="Save"
        secondaryButtonLabel="Cancel"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
      />
    );

    fireEvent.click(screen.getByText("Save"));
    expect(mockOnPrimaryAction).toHaveBeenCalledTimes(1);
  });

  it("calls onSecondaryAction when secondary button is clicked", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Modal"
        primaryButtonLabel="Save"
        secondaryButtonLabel="Cancel"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
        onSecondaryAction={mockOnSecondaryAction}
      />
    );

    fireEvent.click(screen.getByText("Cancel"));
    expect(mockOnSecondaryAction).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when secondary button is clicked and onSecondaryAction is not provided", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Modal"
        primaryButtonLabel="Save"
        secondaryButtonLabel="Cancel"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
      />
    );

    fireEvent.click(screen.getByText("Cancel"));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("renders children correctly", () => {
    renderWithTheme(
      <ActionModal
        open={true}
        title="Test Modal"
        primaryButtonLabel="Save"
        secondaryButtonLabel="Cancel"
        onClose={mockOnClose}
        onPrimaryAction={mockOnPrimaryAction}
      >
        <div>Modal Content</div>
      </ActionModal>
    );

    expect(screen.getByText("Modal Content")).toBeInTheDocument();
  });
}); 