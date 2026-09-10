import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../utils/testTheme";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SubmissionReview from "./SubmissionReview";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const apiClient = require("../utils/apiClient").default;

const renderWithRoute = (submissionId = "1") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[`/admin/submissions/${submissionId}`]}>
        <Routes>
          <Route path="/admin/submissions/:id" element={<SubmissionReview />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const mockSubmission = {
  id: 1,
  resource_type: "datasource",
  status: "submitted",
  is_update: false,
  resource_payload: {
    name: "Community Vector DB",
    short_description: "Product embeddings",
    db_source_type: "pgvector",
    embed_vendor: "openai",
    embed_model: "text-embedding-3-small",
  },
  suggested_privacy: 50,
  privacy_justification: "Public product data",
  primary_contact: "dev@company.com",
  notes: "First version",
  submitter: { name: "Dev User", email: "dev@company.com" },
  submitter_id: 2,
  submitted_at: "2024-01-15T10:00:00Z",
  resource_id: null,
};

describe("SubmissionReview", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders submission title and status", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Review: Community Vector DB")).toBeInTheDocument();
      expect(screen.getByText("Pending Review")).toBeInTheDocument();
    });
  });

  it("shows claim review banner for submitted status", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Claim Review")).toBeInTheDocument();
    });
  });

  it("shows resource configuration details", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Resource Configuration")).toBeInTheDocument();
      expect(screen.getByText("pgvector")).toBeInTheDocument();
    });
  });

  it("shows action buttons for reviewable submissions", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Approve")).toBeInTheDocument();
      expect(screen.getByText("Request Changes")).toBeInTheDocument();
      expect(screen.getByText("Reject")).toBeInTheDocument();
    });
  });

  it("hides action buttons for approved submissions", async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: { ...mockSubmission, status: "approved", resource_id: 42 } },
    });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Review: Community Vector DB")).toBeInTheDocument();
    });
    expect(screen.queryByText("Approve")).not.toBeInTheDocument();
  });

  // The endpoint does two materially different things: a datasource submission
  // genuinely contacts the embedding service, a tool submission only re-parses
  // its spec. One label for both promised the stronger guarantee, so a reviewer
  // could approve a tool whose server block points nowhere.
  it("offers to test the connection for a datasource", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Test connection")).toBeInTheDocument();
    });
  });

  it("offers only to validate the specification for a tool", async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: { ...mockSubmission, resource_type: "tool" } },
    });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Validate specification")).toBeInTheDocument();
    });
    expect(screen.queryByText("Test connection")).not.toBeInTheDocument();
  });

  it("renders the attestations the submitter accepted", async () => {
    // Stored and returned by the API all along, and never rendered -- so the
    // reviewer approved without ever seeing the legal gate they configured.
    apiClient.get
      .mockResolvedValueOnce({
        data: {
          data: {
            ...mockSubmission,
            attestations: {
              accepted: [
                { template_id: 3, accepted_at: "2024-01-15T10:00:00Z" },
              ],
            },
          },
        },
      })
      .mockResolvedValueOnce({
        data: { data: [{ id: 3, name: "Data Authority", text: "I have the right to share this data." }] },
      });

    renderWithRoute();

    await waitFor(() => {
      expect(screen.getByText("Attestations")).toBeInTheDocument();
      expect(screen.getByText("Data Authority")).toBeInTheDocument();
      expect(
        screen.getByText("I have the right to share this data.")
      ).toBeInTheDocument();
    });
  });

  it("says so plainly when no attestations were accepted", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(
        screen.getByText("The submitter accepted no attestations.")
      ).toBeInTheDocument();
    });
  });

  it("shows submitter and privacy info", async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: mockSubmission } });
    renderWithRoute();
    await waitFor(() => {
      expect(screen.getByText("Dev User")).toBeInTheDocument();
      expect(screen.getByText("Public product data")).toBeInTheDocument();
    });
  });

  describe("plugin resource submissions", () => {
    const pluginSubmission = {
      ...mockSubmission,
      resource_type: "plugin",
      plugin_resource_type_id: 7,
      plugin_instance_id: null,
      plugin_resource_type: {
        id: 7,
        plugin_id: 3,
        slug: "agent",
        name: "Agent",
        plugin_name: "Asset Catalog",
        is_active: true,
        submission_schema: {
          type: "object",
          required: ["name"],
          properties: {
            name: { type: "string", title: "Agent Name" },
            system_prompt: { type: "string", title: "System Prompt" },
          },
        },
      },
      resource_payload: {
        name: "Support Bot",
        system_prompt: "Be helpful.",
        custom_flag: true,
      },
    };

    it("labels the type by the plugin resource type and renders the schema-driven payload", async () => {
      apiClient.get.mockResolvedValueOnce({ data: { data: pluginSubmission } });
      renderWithRoute();
      await waitFor(() => {
        expect(screen.getByText("Review: Support Bot")).toBeInTheDocument();
      });
      expect(screen.getByText("Agent")).toBeInTheDocument();
      expect(screen.getByText("Asset Catalog")).toBeInTheDocument();
      expect(screen.getByText("Agent Name")).toBeInTheDocument();
      expect(screen.getByText("System Prompt")).toBeInTheDocument();
      expect(screen.getByText("Be helpful.")).toBeInTheDocument();
      expect(screen.getByText("custom_flag")).toBeInTheDocument();
    });

    it("offers no connectivity test, since there is nothing to test", async () => {
      apiClient.get.mockResolvedValueOnce({ data: { data: pluginSubmission } });
      renderWithRoute();
      await waitFor(() => {
        expect(screen.getByText("Resource Configuration")).toBeInTheDocument();
      });
      expect(screen.queryByText("Test connection")).not.toBeInTheDocument();
      expect(
        screen.queryByText("Validate specification")
      ).not.toBeInTheDocument();
    });

    it("approves with an empty catalogue assignment", async () => {
      apiClient.get.mockResolvedValueOnce({ data: { data: pluginSubmission } });
      apiClient.post.mockResolvedValue({ data: { data: {} } });
      renderWithRoute();
      await waitFor(() => {
        expect(screen.getByText("Approve")).toBeInTheDocument();
      });
      fireEvent.click(screen.getByText("Approve"));
      // The dialog's confirm button carries the same label as the opener.
      const confirm = (await screen.findAllByRole("button", { name: "Approve" })).pop();
      fireEvent.click(confirm);

      await waitFor(() => {
        expect(apiClient.post).toHaveBeenCalledWith(
          "/submissions/1/approve",
          expect.objectContaining({
            data: {
              attributes: expect.objectContaining({
                assigned_catalogues: {},
              }),
            },
          })
        );
      });
    });

    it("shows the plugin resource id once approved", async () => {
      apiClient.get.mockResolvedValueOnce({
        data: {
          data: {
            ...pluginSubmission,
            status: "approved",
            plugin_instance_id: "agent-9f3c",
          },
        },
      });
      renderWithRoute();
      await waitFor(() => {
        expect(screen.getByText("agent-9f3c")).toBeInTheDocument();
      });
    });
  });
});
