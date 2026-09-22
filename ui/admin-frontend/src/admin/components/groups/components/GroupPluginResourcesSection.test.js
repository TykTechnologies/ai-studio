import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import GroupPluginResourcesSection from "./GroupPluginResourcesSection";
import apiClient from "../../../utils/apiClient";

jest.mock("../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

// The picker is stubbed to expose the per-type helper text it is handed.
jest.mock("../../common/relationship-picker", () => ({
  __esModule: true,
  default: (props) => (
    <div data-testid="relationship-picker" aria-label={props.label}>
      {props.helperText && <p data-testid="picker-helper">{props.helperText}</p>}
    </div>
  ),
}));

const APP_GRANTED_TEXT = "Members will only be able to select these resources when creating apps.";
const PLUGIN_GRANTED_TEXT =
  "Members can see these resources in the portal. They are not selectable when creating apps; access is granted by the plugin itself.";

const types = [
  { plugin_id: 7, slug: "vector-store", name: "Vector stores", access_granted_via_app: true },
  { plugin_id: 9, slug: "assets", name: "Catalog assets", access_granted_via_app: false },
];

describe("GroupPluginResourcesSection helper text", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) => {
      if (url === "/plugin-resource-types") return Promise.resolve({ data: { data: types } });
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it("explains per type whether members pick the resources in apps or get access from the plugin", async () => {
    render(<GroupPluginResourcesSection groupId="5" onChange={jest.fn()} />);

    const appPicker = await screen.findByLabelText("Vector stores");
    expect(appPicker).toHaveTextContent(APP_GRANTED_TEXT);
    expect(appPicker).not.toHaveTextContent(PLUGIN_GRANTED_TEXT);

    const pluginPicker = screen.getByLabelText("Catalog assets");
    expect(pluginPicker).toHaveTextContent(PLUGIN_GRANTED_TEXT);
    expect(pluginPicker).not.toHaveTextContent(APP_GRANTED_TEXT);

    // The section header no longer claims every type is app-selectable.
    expect(screen.getByText("Assign plugin resource instances to this team.")).toBeInTheDocument();
    expect(screen.getAllByText(APP_GRANTED_TEXT)).toHaveLength(1);
  });

  it("treats a type without the flag as app-granted (the legacy default)", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/plugin-resource-types") {
        return Promise.resolve({ data: { data: [{ plugin_id: 3, slug: "legacy", name: "Legacy" }] } });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    render(<GroupPluginResourcesSection groupId="5" onChange={jest.fn()} />);
    expect(await screen.findByLabelText("Legacy")).toHaveTextContent(APP_GRANTED_TEXT);
  });
});
