import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../utils/testTheme";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SubmissionReview from "./SubmissionReview";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const apiClient = require("../utils/apiClient").default;

const submission = {
  id: 5,
  resource_type: "mcp_server",
  status: "submitted",
  is_update: false,
  resource_payload: {
    name: "Weather MCP",
    description: "Weather for agents",
    kind: "remote",
    connection_id: 1,
    upstream_url: "https://weather.example.com/mcp",
    upstream_auth_header_name: "X-Token",
    upstream_auth_token: "[redacted]",
    consumer_auth: "auth_token",
    suggested_listen_path: "/weather-mcp/",
    gateway_tags: ["edge-eu"],
  },
  suggested_privacy: 30,
  privacy_justification: "public data",
  primary_contact: "member@tyk.io",
  submitter: { name: "Member", email: "member@tyk.io" },
  submitter_id: 2,
  submitted_at: "2026-09-16T10:00:00Z",
  resource_id: null,
  attestations: { accepted: [] },
};

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/submissions/5"]}>
        <Routes>
          <Route path="/admin/submissions/:id" element={<SubmissionReview />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("SubmissionReview for MCP servers", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) => {
      if (url === "/submissions/5") return Promise.resolve({ data: { data: submission } });
      if (url === "/tyk-connections/1/gateway-tags") return Promise.resolve({ data: [{ tag: "edge-eu", verified: true }, { tag: "edge-us", verified: false }] });
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it("shows the MCP payload with the credential redacted and validates on the Dashboard", async () => {
    apiClient.post.mockResolvedValue({ data: { data: { type: "mcp_server", check_kind: "spec_validation", check_label: "Definition rendered and checked", check_detail: "The upstream MCP server was not contacted." } } });
    renderPage();
    const summary = await screen.findByTestId("mcp-submission-summary");
    expect(summary).toHaveTextContent("https://weather.example.com/mcp");
    expect(summary).toHaveTextContent("X-Token: [redacted]");
    expect(summary).toHaveTextContent("edge-eu");
    fireEvent.click(screen.getByText("Validate definition"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/submissions/5/test"));
    expect(await screen.findByText("Definition rendered and checked")).toBeInTheDocument();
  });

  it("sends the reviewer's deployment target and publish choice on approve", async () => {
    apiClient.post.mockResolvedValue({ data: { data: { ...submission, status: "approved" } } });
    renderPage();
    await screen.findByTestId("mcp-submission-summary");
    fireEvent.click(screen.getByRole("button", { name: /^Approve$/ }));
    await screen.findByTestId("mcp-approve-options");
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/tyk-connections/1/gateway-tags"));
    fireEvent.click(await screen.findByTestId("approve-publish"));
    const buttons = screen.getAllByRole("button", { name: /^Approve$/ });
    fireEvent.click(buttons[buttons.length - 1]);
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/submissions/5/approve", {
        data: { attributes: expect.objectContaining({ gateway_tags: ["edge-eu"], publish: true }) },
      })
    );
    expect(mockNavigate).toHaveBeenCalledWith("/admin/submissions", expect.objectContaining({ state: expect.objectContaining({ snackbar: expect.objectContaining({ message: expect.stringContaining("MCP server") }) }) }));
  });
});
