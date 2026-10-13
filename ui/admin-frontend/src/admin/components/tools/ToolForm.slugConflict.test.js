import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import ToolForm from "./ToolForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import { usePermissions } from "../../context/PermissionsContext";
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
} from "../../services/governedMetadataService";

// #785: a tool name whose slug another tool already has is refused with 409
// and a detail; the form shows it under Name rather than "Failed to save tool".

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../context/PermissionsContext", () => ({ usePermissions: jest.fn() }));
jest.mock("../../hooks/useSystemFeatures", () => ({ __esModule: true, default: () => ({ features: {} }) }));
jest.mock("../../services/governedMetadataService", () => ({
  ...jest.requireActual("../../services/governedMetadataService"),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
  validateObjectMetadata: jest.fn(),
}));
jest.mock("../../../components/common/Icon", () => function MockIcon(props) {
  return <div data-testid="mock-icon">{props.name}</div>;
});
jest.mock("../common/relationship-picker", () =>
  require("../../../test-utils/component-mocks").relationshipPickerMock,
);
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
  useParams: () => ({}),
}));

const DETAIL =
  'tool name "Page Fetcher" is too close to tool 2 ("page-fetcher"): both have the slug "page-fetcher", and tool names must differ by more than case, spaces or punctuation';

describe("ToolForm slug conflict", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: false });
    usePermissions.mockReturnValue({ can: () => true });
    apiClient.get.mockImplementation((url) =>
      Promise.resolve(url === "/filters" ? { data: [] } : { data: { data: [] } })
    );
    apiClient.post.mockRejectedValue({ response: { status: 409, data: { errors: [{ title: "Conflict", detail: DETAIL }] } } });
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("shows the conflict under the Name field and focuses it", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <ToolForm />
        </MemoryRouter>
      </ThemeProvider>
    );
    await screen.findByRole("button", { name: "Add tool" });
    const name = screen.getByLabelText(/^Name/);
    fireEvent.change(name, { target: { value: "Page Fetcher" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Fetches pages" } });
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));

    await waitFor(() => expect(name).toHaveAttribute("aria-invalid", "true"));
    expect(screen.getAllByText(DETAIL).length).toBeGreaterThan(0);
    expect(screen.queryByText("Failed to save tool. Please try again.")).not.toBeInTheDocument();
    await waitFor(() => expect(name).toHaveFocus());
  });
});
