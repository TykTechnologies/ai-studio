import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import AppList from "./AppList";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../components/rbac/Can", () => ({ children }) => <>{children}</>);
jest.mock("../../components/common/Icon", () => (props) => <div data-testid="mock-icon">{props.name}</div>);
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const apps = [
  { id: "1", attributes: { name: "Approved App", description: "", user_id: 1, credential_id: 11 } },
  { id: "2", attributes: { name: "Waiting App", description: "", user_id: 1, credential_id: 12 } },
  { id: "3", attributes: { name: "Bare App", description: "", user_id: 1, credential_id: null } },
];

const renderList = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <AppList />
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("AppList status labels and approval", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps") return Promise.resolve({ data: { data: apps }, headers: { "x-total-count": "3", "x-total-pages": "1" } });
      if (url === "/credentials")
        return Promise.resolve({
          data: {
            data: [
              { id: "11", attributes: { active: true } },
              { id: "12", attributes: { active: false } },
            ],
          },
        });
      if (url === "/users") return Promise.resolve({ data: { data: [{ id: "1", attributes: { name: "Ada" } }] } });
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
    apiClient.patch.mockResolvedValue({ data: {} });
  });

  const rowFor = async (name) => (await screen.findByText(name)).closest("tr");

  it("uses the Approved / Awaiting approval / No credential labels", async () => {
    renderList();
    await waitFor(() => expect(within(screen.getByText("Approved App").closest("tr")).getByText("Approved")).toBeInTheDocument());
    expect(within(await rowFor("Waiting App")).getByText("Awaiting approval")).toBeInTheDocument();
    expect(within(await rowFor("Bare App")).getByText("No credential")).toBeInTheDocument();
    expect(screen.queryByText("Inactive")).not.toBeInTheDocument();
    expect(screen.queryByText("Pending")).not.toBeInTheDocument();
  });

  it("offers Approve credentials only for apps awaiting approval and activates the credential", async () => {
    renderList();
    const waiting = await rowFor("Waiting App");
    await waitFor(() => expect(within(waiting).getByText("Awaiting approval")).toBeInTheDocument());
    fireEvent.click(within(waiting).getByRole("button"));
    fireEvent.click(await screen.findByRole("menuitem", { name: /Approve credentials/ }));
    await waitFor(() =>
      expect(apiClient.patch).toHaveBeenCalledWith("/credentials/12", {
        data: { type: "credentials", attributes: { active: true } },
      }),
    );
    expect(await screen.findByText("App credentials approved")).toBeInTheDocument();
  });

  it("does not offer Approve credentials on an approved app", async () => {
    renderList();
    const approved = await rowFor("Approved App");
    await waitFor(() => expect(within(approved).getByText("Approved")).toBeInTheDocument());
    fireEvent.click(within(approved).getByRole("button"));
    await screen.findByRole("menuitem", { name: /Disable credentials/ });
    expect(screen.queryByRole("menuitem", { name: /Approve credentials/ })).not.toBeInTheDocument();
  });
});
