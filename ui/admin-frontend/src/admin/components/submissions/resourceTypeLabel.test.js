import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import {
  getResourceTypeLabel,
  getResourceTypeIcon,
  ResourceTypeChip,
} from "./resourceTypeLabel";

describe("getResourceTypeLabel", () => {
  it("labels the built-in types", () => {
    expect(getResourceTypeLabel({ resource_type: "datasource" })).toBe(
      "Data Source"
    );
    expect(getResourceTypeLabel({ resource_type: "tool" })).toBe("Tool");
  });

  it("uses the plugin resource type's name for plugin submissions", () => {
    expect(
      getResourceTypeLabel({
        resource_type: "plugin",
        plugin_resource_type: { id: 3, name: "Agent", slug: "agent" },
      })
    ).toBe("Agent");
  });

  it("falls back to a generic label when the plugin type is not embedded", () => {
    expect(getResourceTypeLabel({ resource_type: "plugin" })).toBe(
      "Plugin Resource"
    );
    expect(
      getResourceTypeLabel({ resource_type: "plugin", plugin_resource_type: {} })
    ).toBe("Plugin Resource");
  });

  it("tolerates a missing submission", () => {
    expect(getResourceTypeLabel(undefined)).toBe("Resource");
    expect(getResourceTypeIcon(undefined)).toBeTruthy();
  });
});

describe("ResourceTypeChip", () => {
  it("renders the plugin type name and the extension icon", () => {
    render(
      <ResourceTypeChip
        submission={{
          resource_type: "plugin",
          plugin_resource_type: { name: "Prompt" },
        }}
      />
    );
    expect(screen.getByText("Prompt")).toBeInTheDocument();
    expect(screen.getByTestId("ExtensionIcon")).toBeInTheDocument();
  });

  it("omits the icon when asked", () => {
    render(
      <ResourceTypeChip
        submission={{ resource_type: "tool" }}
        withIcon={false}
      />
    );
    expect(screen.getByText("Tool")).toBeInTheDocument();
    expect(screen.queryByTestId("BuildIcon")).not.toBeInTheDocument();
  });
});
