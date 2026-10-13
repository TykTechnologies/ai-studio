import React from "react";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../utils/testTheme";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SubmissionReview from "./SubmissionReview";

// #504: the Approve dialog lets the reviewer choose which catalogues an
// approved tool or data source joins, sent as assigned_catalogues.

let mockFeatures = { feature_groups: true };
jest.mock("../hooks/useSystemFeatures", () => () => ({ features: mockFeatures }));
jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("../utils/listAll", () => ({
  __esModule: true,
  listAll: jest.fn(),
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const apiClient = require("../utils/apiClient").default;
const { listAll } = require("../utils/listAll");

const submission = (resource_type) => ({
  id: 1,
  resource_type,
  status: "in_review",
  is_update: false,
  resource_payload: { name: "Weather", tool_type: "REST" },
  suggested_privacy: 20,
  submitter: { name: "Dev", email: "dev@x" },
  submitter_id: 2,
  resource_id: null,
});

const catalogues = {
  data: {
    data: [
      { id: "1", attributes: { name: "Default" } },
      { id: "4", attributes: { name: "Community" } },
      { id: "9", attributes: { name: "Finance" } },
    ],
  },
};

const renderReview = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/submissions/1"]}>
        <Routes>
          <Route path="/admin/submissions/:id" element={<SubmissionReview />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const openApproveDialog = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "Approve" }));
  return screen.findByRole("dialog");
};

const confirmApprove = async (dialog) => {
  fireEvent.click(within(dialog).getByRole("button", { name: "Approve" }));
  await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
  return apiClient.post.mock.calls[0][1].data.attributes;
};

describe("Approve dialog: catalogue picker (#504)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockFeatures = { feature_groups: true };
    apiClient.get.mockImplementation((url) =>
      Promise.resolve(url === "/submissions/1" ? { data: { data: submission("tool") } } : { data: { data: [] } })
    );
    apiClient.post.mockResolvedValue({ data: { data: {} } });
    // /llms feeds the final-score privacy warning; no providers there.
    listAll.mockImplementation((client, path) => Promise.resolve(path === "/llms" ? { data: { data: [] } } : catalogues));
  });

  it("loads tool catalogues with listAll, preselects Default and sends it by default", async () => {
    renderReview();
    const dialog = await openApproveDialog();
    expect(await within(dialog).findByRole("button", { name: "Default" })).toBeInTheDocument();
    expect(listAll).toHaveBeenCalledWith(apiClient, "/tool-catalogues");
    // The consequence of the preselection is spelled out before Approve.
    expect(within(dialog).getByTestId("approve-catalogues-consequence")).toHaveTextContent(
      "Every team that has Default can use the resource once it is active"
    );

    const attrs = await confirmApprove(dialog);
    expect(attrs.assigned_catalogues).toEqual({ ids: [1] });
    expect(mockNavigate.mock.calls[0][1].state.snackbar.message).toMatch(/added to Default/);
  });

  it("sends exactly the catalogues the reviewer picked", async () => {
    renderReview();
    const dialog = await openApproveDialog();
    await within(dialog).findByRole("button", { name: "Default" });

    fireEvent.mouseDown(within(dialog).getByTestId("approve-catalogues"));
    fireEvent.click(await screen.findByRole("option", { name: "Finance" }));

    const attrs = await confirmApprove(dialog);
    expect(attrs.assigned_catalogues).toEqual({ ids: [1, 9] });
  });

  it("sends an empty list when the reviewer clears the picker", async () => {
    renderReview();
    const dialog = await openApproveDialog();
    const chip = await within(dialog).findByRole("button", { name: "Default" });
    fireEvent.keyDown(chip, { key: "Backspace" });
    await waitFor(() => expect(within(dialog).queryByRole("button", { name: "Default" })).not.toBeInTheDocument());
    expect(within(dialog).getByTestId("approve-catalogues-consequence")).toHaveTextContent(
      "approved without being published to any catalog"
    );

    const attrs = await confirmApprove(dialog);
    expect(attrs.assigned_catalogues).toEqual({ ids: [] });
    expect(mockNavigate.mock.calls[0][1].state.snackbar.message).toMatch(/no catalog/);
  });

  it("uses data catalogues for a data source submission", async () => {
    apiClient.get.mockImplementation((url) =>
      Promise.resolve(
        url === "/submissions/1" ? { data: { data: submission("datasource") } } : { data: { data: [] } }
      )
    );
    renderReview();
    const dialog = await openApproveDialog();
    await within(dialog).findByRole("button", { name: "Default" });
    expect(listAll).toHaveBeenCalledWith(apiClient, "/data-catalogues");
    expect(within(dialog).getByLabelText("Publish to data catalogs")).toBeInTheDocument();
  });

  it("offers no picker in Community Edition, which always publishes to Default", async () => {
    mockFeatures = { feature_groups: false };
    renderReview();
    const dialog = await openApproveDialog();
    expect(within(dialog).queryByTestId("approve-catalogues")).not.toBeInTheDocument();
    expect(listAll).not.toHaveBeenCalledWith(apiClient, "/tool-catalogues");

    const attrs = await confirmApprove(dialog);
    expect(attrs).not.toHaveProperty("assigned_catalogues");
  });

  // Security review: an update changes a resource that already exists (and
  // may be live); approving it must not add that resource to Default.
  it("offers no picker and sends no catalogues for an update submission", async () => {
    apiClient.get.mockImplementation((url) =>
      Promise.resolve(
        url === "/submissions/1"
          ? { data: { data: { ...submission("tool"), is_update: true, target_resource_id: 12 } } }
          : { data: { data: [] } }
      )
    );
    renderReview();
    const dialog = await openApproveDialog();
    expect(within(dialog).queryByTestId("approve-catalogues")).not.toBeInTheDocument();
    expect(listAll).not.toHaveBeenCalledWith(apiClient, "/tool-catalogues");

    const attrs = await confirmApprove(dialog);
    expect(attrs).not.toHaveProperty("assigned_catalogues");
    expect(mockNavigate.mock.calls[0][1].state.snackbar.message).toMatch(/applied to the existing resource/);
  });
});
