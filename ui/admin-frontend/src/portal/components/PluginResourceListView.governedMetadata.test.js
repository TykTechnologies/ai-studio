import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import adminTheme from "../../admin/theme";
import PluginResourceListView from "./PluginResourceListView";
import pubClient from "../../admin/utils/pubClient";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const renderView = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={["/portal/resources/12/prompts"]}>
        <Routes>
          <Route path="/portal/resources/:pluginId/:slug" element={<PluginResourceListView />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("PluginResourceListView governed metadata badges", () => {
  beforeEach(() => {
    pubClient.get.mockResolvedValue({
      data: {
        data: [
          {
            plugin_id: 12,
            slug: "prompts",
            name: "Prompts",
            supports_metadata: true,
            instances: [
              {
                id: "ast_1",
                name: "Welcome prompt",
                description: "Greets users",
                privacy_score: 0,
                governed_metadata: [
                  { key: "data_classification", label: "Data classification", type: "vocabulary", value: "Internal" },
                  { key: "lifecycle_state", label: "Lifecycle state", type: "vocabulary", value: "Active" },
                ],
              },
              { id: "ast_2", name: "Refund prompt", description: "", privacy_score: 40 },
            ],
          },
          { plugin_id: 12, slug: "agents", name: "Agents", instances: [{ id: "ag_1", name: "Other type" }] },
        ],
      },
    });
  });

  it("shows badges only for instances that carry portal-visible metadata", async () => {
    renderView();
    expect(await screen.findByText("Welcome prompt")).toBeInTheDocument();
    expect(screen.getByText("Data classification: Internal")).toBeInTheDocument();
    expect(screen.getByText("Lifecycle state: Active")).toBeInTheDocument();
    expect(screen.getAllByTestId("governed-metadata-badges")).toHaveLength(1);
    expect(screen.getByText("Refund prompt")).toBeInTheDocument();
    expect(screen.getByText("Privacy: 40")).toBeInTheDocument();
    expect(screen.queryByText("Other type")).not.toBeInTheDocument();
  });

  it("copes with a type that has no instances", async () => {
    pubClient.get.mockResolvedValue({ data: { data: [{ plugin_id: 12, slug: "prompts", name: "Prompts", instances: [] }] } });
    renderView();
    expect(await screen.findByText(/No Prompts are currently available/)).toBeInTheDocument();
    expect(screen.queryByTestId("governed-metadata-badges")).not.toBeInTheDocument();
  });
});
