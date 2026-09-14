import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import AppListView from "./AppListView";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), delete: jest.fn() },
}));

const pubClient = require("../../admin/utils/pubClient").default;

const listApp = (id, name, isActive = true) => ({
  id,
  type: "app",
  attributes: {
    name,
    description: `${name} description`,
    is_active: isActive,
    llm_ids: [1],
    datasource_ids: [],
    tool_ids: [],
  },
});

const detail = (credentialActive) => ({
  data: {
    id: "x",
    type: "app",
    attributes: { credential: { key_id: "k", secret: "s", active: credentialActive } },
  },
});

const renderView = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <AppListView />
      </MemoryRouter>
    </ThemeProvider>
  );

const rowFor = (name) => screen.getByRole("row", { name: new RegExp(name) });

// The list had no status column: an App awaiting approval looked exactly like
// a live one until a request came back 401 (UX review F-04 / Q4).
describe("AppListView status column", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({ data: { data: [] } });
  });

  it("shows one status per app derived from the app switch and its credential", async () => {
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/apps") {
        return Promise.resolve({
          data: {
            data: [
              listApp("1", "Live app"),
              listApp("2", "Waiting app"),
              listApp("3", "Switched off app", false),
            ],
          },
        });
      }
      if (url === "/common/apps/1") return Promise.resolve(detail(true));
      if (url === "/common/apps/2") return Promise.resolve(detail(false));
      return Promise.reject(new Error(`unexpected ${url}`));
    });

    renderView();

    await waitFor(() => {
      expect(screen.getByRole("columnheader", { name: "Status" })).toBeInTheDocument();
    });
    expect(within(rowFor("Live app")).getByText("Active")).toBeInTheDocument();
    expect(within(rowFor("Waiting app")).getByText("Awaiting approval")).toBeInTheDocument();
    expect(within(rowFor("Switched off app")).getByText("Disabled")).toBeInTheDocument();
    // A disabled App is Disabled whatever its credential says, so its detail
    // (and credential) is never fetched.
    expect(pubClient.get).not.toHaveBeenCalledWith("/common/apps/3");
  });

  it("uses a credential carried by the list without a second request", async () => {
    const app = listApp("9", "Inline app");
    app.attributes.credential = { active: true };
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/apps") return Promise.resolve({ data: { data: [app] } });
      return Promise.reject(new Error(`unexpected ${url}`));
    });

    renderView();

    await waitFor(() => {
      expect(within(rowFor("Inline app")).getByText("Active")).toBeInTheDocument();
    });
    expect(pubClient.get).toHaveBeenCalledTimes(1);
  });

  it("falls back to Awaiting approval when the credential cannot be read", async () => {
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/apps") {
        return Promise.resolve({ data: { data: [listApp("4", "Unknown app")] } });
      }
      return Promise.reject(new Error("boom"));
    });
    jest.spyOn(console, "error").mockImplementation(() => {});

    renderView();

    await waitFor(() => {
      expect(within(rowFor("Unknown app")).getByText("Awaiting approval")).toBeInTheDocument();
    });
  });
});
