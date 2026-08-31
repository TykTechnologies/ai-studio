import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import PushConfigurationModal from "./PushConfigurationModal";

jest.mock("../../hooks/useNamespaces", () => ({
  __esModule: true,
  default: () => ({ getAvailableNamespaces: () => ["global", "team-a"] }),
}));

// Enterprise: namespace selection is available.
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: { hub_spoke_multi_tenant: true } }),
}));

jest.mock("../../context/SyncStatusContext", () => ({
  useSyncStatus: () => ({ refreshSyncStatus: jest.fn() }),
}));

const renderModal = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <PushConfigurationModal open onClose={() => {}} />
    </ThemeProvider>
  );

describe("PushConfigurationModal", () => {
  // On Enterprise this opened defaulted to "Specific Namespace" with nothing
  // selected, so the submit button was already disabled with no indication of
  // why and the user had to work out that a namespace still had to be chosen.
  it("opens in a state it can actually submit", () => {
    renderModal();
    const push = screen.getByRole("button", { name: /push configuration/i });
    expect(push).not.toBeDisabled();
  });
});
