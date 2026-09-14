import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../../admin/utils/testTheme";
import AssetAvatar from "./AssetAvatar";

const renderAvatar = (props) =>
  render(
    <ThemeProvider theme={testTheme}>
      <AssetAvatar {...props} />
    </ThemeProvider>
  );

// The placeholder image made every card the same; assets without a logo get
// initials on a colour derived from their name instead.
describe("AssetAvatar", () => {
  it("shows initials when there is no logo", () => {
    renderAvatar({ name: "Acme OpenAI", seed: "llm:1" });
    const avatar = screen.getByTestId("asset-avatar");
    expect(avatar).toHaveAttribute("data-avatar-mode", "initials");
    expect(avatar).toHaveTextContent("AO");
  });

  it("shows the logo when one is set and falls back to initials if it fails", () => {
    renderAvatar({ name: "Acme OpenAI", seed: "llm:1", logoUrl: "https://example.com/logo.png" });
    const avatar = screen.getByTestId("asset-avatar");
    expect(avatar).toHaveAttribute("data-avatar-mode", "logo");
    fireEvent.error(screen.getByTestId("asset-avatar-logo"));
    expect(avatar).toHaveAttribute("data-avatar-mode", "initials");
    expect(avatar).toHaveTextContent("AO");
  });

  it("gives the same name the same colour every time", () => {
    const { unmount } = renderAvatar({ name: "Weather API", seed: "tool:4" });
    const first = screen.getByTestId("asset-avatar").style.background;
    unmount();
    renderAvatar({ name: "Weather API", seed: "tool:4" });
    expect(screen.getByTestId("asset-avatar").style.background).toBe(first);
  });
});
