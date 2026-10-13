import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SubmissionDetail, { catalogEntryPath } from "./SubmissionDetail";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
  },
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const pubClient = require("../../admin/utils/pubClient").default;

const renderWithRoute = (submissionId = "1") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[`/portal/submissions/${submissionId}`]}>
        <Routes>
          <Route
            path="/portal/submissions/:id"
            element={<SubmissionDetail />}
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const mockSubmission = {
  id: 1,
  resource_type: "datasource",
  status: "approved",
  is_update: false,
  resource_payload: {
    name: "Test Vector DB",
    short_description: "A test datasource",
    db_source_type: "pgvector",
    embed_vendor: "openai",
    embed_model: "text-embedding-3-small",
  },
  suggested_privacy: 50,
  final_privacy_score: 45,
  privacy_justification: "Public data only",
  primary_contact: "dev@company.com",
  documentation_url: "https://docs.example.com",
  submitted_at: "2024-01-15T10:00:00Z",
  review_completed_at: "2024-01-16T15:00:00Z",
  created_at: "2024-01-14T08:00:00Z",
  submitter_feedback: null,
  resource_id: 42,
};

describe("SubmissionDetail", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders submission details", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: mockSubmission },
    });

    renderWithRoute();

    await waitFor(() => {
      expect(screen.getByText("Test Vector DB")).toBeInTheDocument();
      expect(screen.getByText("Approved")).toBeInTheDocument();
    });
  });

  it("shows timeline information", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: mockSubmission },
    });

    renderWithRoute();

    await waitFor(() => {
      expect(screen.getByText("Timeline")).toBeInTheDocument();
    });
  });

  it("shows the plugin payload and published id for a plugin resource", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: {
        data: {
          ...mockSubmission,
          resource_type: "plugin",
          resource_id: null,
          plugin_resource_type_id: 7,
          plugin_instance_id: "agent-9f3c",
          plugin_resource_type: {
            id: 7,
            name: "Agent",
            plugin_name: "Asset Catalog",
            submission_schema: {
              type: "object",
              properties: {
                name: { type: "string", title: "Agent Name" },
                system_prompt: { type: "string", title: "System Prompt" },
              },
            },
          },
          resource_payload: { name: "Support Bot", system_prompt: "Be helpful." },
        },
      },
    });

    renderWithRoute();

    // The name is both the page heading and a payload row.
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Support Bot" })
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("System Prompt")).toBeInTheDocument();
    expect(screen.getByText("Be helpful.")).toBeInTheDocument();
    expect(screen.getByText("agent-9f3c")).toBeInTheDocument();
    expect(screen.queryByText("View in Catalog")).not.toBeInTheDocument();
  });

  it("shows resource configuration for datasource", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: mockSubmission },
    });

    renderWithRoute();

    await waitFor(() => {
      expect(screen.getByText("Resource Configuration")).toBeInTheDocument();
      expect(screen.getByText("pgvector")).toBeInTheDocument();
      expect(screen.getByText("openai")).toBeInTheDocument();
    });
  });

  it("shows admin feedback for changes_requested", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: {
        data: {
          ...mockSubmission,
          status: "changes_requested",
          submitter_feedback: "Please add more documentation",
        },
      },
    });

    renderWithRoute();

    await waitFor(() => {
      expect(
        screen.getByText("Please add more documentation")
      ).toBeInTheDocument();
    });
  });

  it("shows Edit & Resubmit button for changes_requested", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: {
        data: {
          ...mockSubmission,
          status: "changes_requested",
          submitter_feedback: "Fix it",
        },
      },
    });

    renderWithRoute();

    await waitFor(() => {
      expect(screen.getByText("Edit & Resubmit")).toBeInTheDocument();
    });
  });

  it("shows Edit Draft button for draft status", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: {
        data: {
          ...mockSubmission,
          status: "draft",
          submitted_at: null,
        },
      },
    });

    renderWithRoute();

    await waitFor(() => {
      expect(screen.getByText("Edit Draft")).toBeInTheDocument();
    });
  });

  it("shows error state when fetch fails", async () => {
    pubClient.get.mockRejectedValueOnce(new Error("Network error"));

    renderWithRoute();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load submission details")
      ).toBeInTheDocument();
    });
  });

  // #512 C4a: "View in Catalog" opened the dashboard for every submission.
  describe("View in Catalog", () => {
    it("links an approved data source to its catalogue entry", async () => {
      pubClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
      renderWithRoute();
      const link = await screen.findByRole("link", { name: "View in Catalog" });
      expect(link).toHaveAttribute("href", "/portal/catalog/datasources/42");
    });

    it("is absent until the submission is approved", async () => {
      pubClient.get.mockResolvedValueOnce({
        data: { data: { ...mockSubmission, status: "in_review", resource_id: null } },
      });
      renderWithRoute();
      await screen.findByText("Timeline");
      expect(screen.queryByText("View in Catalog")).not.toBeInTheDocument();
    });
  });

  describe("catalogEntryPath", () => {
    const approved = (extra) => ({ status: "approved", ...extra });

    it.each([
      ["tool", approved({ resource_type: "tool", resource_id: 5 }), "/portal/catalog/tools/5"],
      ["data source", approved({ resource_type: "datasource", resource_id: 6 }), "/portal/catalog/datasources/6"],
      ["MCP server", approved({ resource_type: "mcp_server", resource_id: 3 }), "/portal/catalog/mcp-servers/3"],
      [
        "plugin resource",
        approved({
          resource_type: "plugin",
          plugin_instance_id: "agent 9/f",
          plugin_resource_type: { plugin_id: 7, slug: "agents" },
        }),
        "/portal/catalog/resources/7/agents/agent%209%2Ff",
      ],
      [
        "approved update",
        approved({ resource_type: "tool", resource_id: null, is_update: true, target_resource_id: 9 }),
        "/portal/catalog/tools/9",
      ],
    ])("opens the %s it created", (_, submission, path) => {
      expect(catalogEntryPath(submission)).toBe(path);
    });

    it.each([
      ["a draft", { status: "draft", resource_type: "tool", resource_id: 5 }],
      ["a rejected one", { status: "rejected", resource_type: "tool", resource_id: 5 }],
      ["an approved one with no resource yet", approved({ resource_type: "tool", resource_id: null })],
      ["a plugin resource without its type", approved({ resource_type: "plugin", plugin_instance_id: "x" })],
      ["an unknown type", approved({ resource_type: "widget", resource_id: 1 })],
      ["nothing", null],
    ])("has no target for %s", (_, submission) => {
      expect(catalogEntryPath(submission)).toBeNull();
    });
  });
});
