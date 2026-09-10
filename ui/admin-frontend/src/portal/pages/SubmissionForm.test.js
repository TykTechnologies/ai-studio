import React from "react";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import SubmissionForm from "./SubmissionForm";

// Mock react-markdown (ESM module that Jest can't handle)
jest.mock("react-markdown", () => ({
  __esModule: true,
  default: ({ children }) => <div data-testid="markdown">{children}</div>,
}));

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
  },
}));

jest.mock("../../admin/utils/vendorUtils", () => ({
  fetchVendors: jest.fn().mockResolvedValue({
    embedders: [{ code: "openai" }, { code: "ollama" }],
    vectorStores: [{ code: "pgvector" }, { code: "chroma" }],
  }),
  getEmbedderDefaultModel: jest.fn().mockReturnValue("text-embedding-3-small"),
  getEmbedderDefaultUrl: jest.fn().mockReturnValue("https://api.openai.com/v1"),
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  // A jest.fn so the edit-mode test can hand the form an id.
  useParams: jest.fn(() => ({})),
}));

const pubClient = require("../../admin/utils/pubClient").default;

const renderWithProviders = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>
  );

describe("SubmissionForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // CRA runs Jest with resetMocks, which strips the factory implementation.
    require("react-router-dom").useParams.mockReturnValue({});
    pubClient.get.mockResolvedValue({ data: { data: [] } });
    // Re-set vendorUtils mock after clearAllMocks
    const { fetchVendors } = require("../../admin/utils/vendorUtils");
    fetchVendors.mockResolvedValue({
      embedders: [{ code: "openai" }, { code: "ollama" }],
      vectorStores: [{ code: "pgvector" }, { code: "chroma" }],
    });
  });

  it("renders the page title", async () => {
    renderWithProviders(<SubmissionForm />);
    await waitFor(() => {
      expect(screen.getByText("Submit a Resource")).toBeInTheDocument();
    });
  });

  it("shows resource type selector label", async () => {
    renderWithProviders(<SubmissionForm />);
    await waitFor(() => {
      const elements = screen.getAllByText("Resource Type");
      expect(elements.length).toBeGreaterThan(0);
    });
  });

  it("fetches attestation templates on load", async () => {
    renderWithProviders(<SubmissionForm />);

    await waitFor(() => {
      expect(pubClient.get).toHaveBeenCalledWith(
        "/common/submissions/attestation-templates"
      );
    });
  });

  it("shows back navigation", () => {
    renderWithProviders(<SubmissionForm />);
    expect(
      screen.getByText("Back to My Contributions")
    ).toBeInTheDocument();
  });
});

describe("SubmissionForm with plugin resource types", () => {
  const agentType = {
    id: 7,
    plugin_id: 3,
    plugin_name: "Asset Catalog",
    slug: "agent",
    name: "Agent",
    description: "A conversational agent backed by a prompt.",
    icon: "smart_toy",
    has_privacy_score: true,
    submission_schema: {
      type: "object",
      required: ["name", "system_prompt"],
      properties: {
        name: { type: "string", title: "Agent Name" },
        system_prompt: { type: "string", title: "System Prompt" },
      },
    },
  };
  const promptType = {
    id: 8,
    plugin_id: 3,
    plugin_name: "Asset Catalog",
    slug: "prompt",
    name: "Prompt",
    description: "A reusable prompt template.",
    has_privacy_score: false,
  };

  const mockGet = (overrides = {}) => {
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/plugin-resource-types") {
        return Promise.resolve({ data: { data: [agentType, promptType] } });
      }
      if (overrides[url]) return Promise.resolve(overrides[url]);
      return Promise.resolve({ data: { data: [] } });
    });
  };

  const openTypeSelect = async () => {
    // Options only exist once the plugin types have loaded.
    await waitFor(() => {
      expect(pubClient.get).toHaveBeenCalledWith(
        "/common/plugin-resource-types"
      );
    });
    fireEvent.mouseDown(
      screen.getByRole("combobox", { name: /resource type/i })
    );
    return screen.findByRole("listbox");
  };

  beforeEach(() => {
    jest.clearAllMocks();
    require("react-router-dom").useParams.mockReturnValue({});
    mockGet();
    const { fetchVendors } = require("../../admin/utils/vendorUtils");
    fetchVendors.mockResolvedValue({ embedders: [], vectorStores: [] });
  });

  it("lists plugin types next to the built-in ones, with the plugin name", async () => {
    renderWithProviders(<SubmissionForm />);
    const listbox = await openTypeSelect();

    expect(within(listbox).getByText("Data Source")).toBeInTheDocument();
    expect(within(listbox).getByText("Tool (OpenAPI)")).toBeInTheDocument();
    expect(within(listbox).getByText("Agent")).toBeInTheDocument();
    expect(within(listbox).getByText("Prompt")).toBeInTheDocument();
    expect(within(listbox).getAllByText("Asset Catalog")).toHaveLength(2);
  });

  it("switches to the schema-driven form when a plugin type is chosen", async () => {
    renderWithProviders(<SubmissionForm />);
    const listbox = await openTypeSelect();
    fireEvent.click(within(listbox).getByText("Agent"));

    // The type's description and the schema's own fields appear...
    expect(
      await screen.findByText("A conversational agent backed by a prompt.")
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/Agent Name/)).toBeInTheDocument();
    expect(screen.getByLabelText(/System Prompt/)).toBeInTheDocument();

    // ...and the generic Basic Information section does not render twice.
    expect(screen.queryByText("Basic Information")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/^Name/)).not.toBeInTheDocument();

    // Shared sections are still there.
    expect(screen.getByText("Privacy & Governance")).toBeInTheDocument();
    expect(screen.getByText("Support & Documentation")).toBeInTheDocument();
  });

  it("falls back to a name field plus JSON when the type has no schema", async () => {
    renderWithProviders(<SubmissionForm />);
    const listbox = await openTypeSelect();
    fireEvent.click(within(listbox).getByText("Prompt"));

    expect(
      await screen.findByText("A reusable prompt template.")
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/^Name/)).toBeInTheDocument();
    expect(
      screen.getByLabelText(/Additional fields \(JSON\)/)
    ).toBeInTheDocument();
  });

  it("sends plugin_resource_type_id and the schema payload on save", async () => {
    pubClient.post.mockResolvedValue({ data: { data: { id: 42 } } });
    renderWithProviders(<SubmissionForm />);
    const listbox = await openTypeSelect();
    fireEvent.click(within(listbox).getByText("Agent"));

    fireEvent.change(await screen.findByLabelText(/Agent Name/), {
      target: { value: "Support Bot" },
    });
    fireEvent.change(screen.getByLabelText(/System Prompt/), {
      target: { value: "Be helpful." },
    });
    fireEvent.click(screen.getByText("Save Draft"));

    await waitFor(() => {
      expect(pubClient.post).toHaveBeenCalledWith(
        "/common/submissions",
        expect.objectContaining({
          data: {
            attributes: expect.objectContaining({
              resource_type: "plugin",
              plugin_resource_type_id: 7,
              status: "draft",
              resource_payload: expect.objectContaining({
                name: "Support Bot",
                system_prompt: "Be helpful.",
              }),
            }),
          },
        })
      );
    });
  });

  it("blocks save when the schema's required fields are missing", async () => {
    renderWithProviders(<SubmissionForm />);
    const listbox = await openTypeSelect();
    fireEvent.click(within(listbox).getByText("Agent"));

    fireEvent.change(await screen.findByLabelText(/Agent Name/), {
      target: { value: "Support Bot" },
    });
    fireEvent.click(screen.getByText("Save Draft"));

    // The validator names the field by its schema title; the message lands in
    // the inline alert above the schema form.
    expect(
      await screen.findByText(/must have required property 'System Prompt'/, {
        selector: ".MuiAlert-message p",
      })
    ).toBeInTheDocument();
    expect(pubClient.post).not.toHaveBeenCalled();
  });

  it("surfaces server-side schema violations from a 400", async () => {
    pubClient.post.mockRejectedValue({
      response: {
        status: 400,
        data: {
          errors: [
            { status: "400", detail: "system_prompt: must be at least 10 characters" },
          ],
        },
      },
    });
    renderWithProviders(<SubmissionForm />);
    const listbox = await openTypeSelect();
    fireEvent.click(within(listbox).getByText("Agent"));

    fireEvent.change(await screen.findByLabelText(/Agent Name/), {
      target: { value: "Support Bot" },
    });
    fireEvent.change(screen.getByLabelText(/System Prompt/), {
      target: { value: "Short" },
    });
    fireEvent.click(screen.getByText("Save Draft"));

    // Snackbar and the inline alert both carry the server's message.
    expect(
      (
        await screen.findAllByText(
          "system_prompt: must be at least 10 characters"
        )
      ).length
    ).toBeGreaterThan(0);
  });

  it("restores a plugin draft with its embedded schema when editing", async () => {
    const { useParams } = require("react-router-dom");
    useParams.mockReturnValue({ id: "42" });
    mockGet({
      "/common/submissions/42": {
        data: {
          data: {
            id: 42,
            resource_type: "plugin",
            plugin_resource_type_id: 7,
            plugin_resource_type: {
              id: 7,
              name: "Agent",
              plugin_name: "Asset Catalog",
              submission_schema: agentType.submission_schema,
            },
            status: "draft",
            resource_payload: { name: "Draft Bot", system_prompt: "Hi" },
            suggested_privacy: 30,
          },
        },
      },
    });
    pubClient.patch.mockResolvedValue({ data: { data: {} } });

    renderWithProviders(<SubmissionForm />);

    expect(await screen.findByDisplayValue("Draft Bot")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Hi")).toBeInTheDocument();
    expect(screen.getByText("Edit Submission")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Save Draft"));
    await waitFor(() => {
      expect(pubClient.patch).toHaveBeenCalledWith(
        "/common/submissions/42",
        expect.objectContaining({
          data: {
            attributes: expect.objectContaining({
              resource_type: "plugin",
              plugin_resource_type_id: 7,
            }),
          },
        })
      );
    });
  });
});
