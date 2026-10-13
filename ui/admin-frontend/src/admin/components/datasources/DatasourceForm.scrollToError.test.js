import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import DatasourceForm from "./DatasourceForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";

// #502 E8: the submit button sits below the fold, so a failed submit must
// bring the first invalid control into view and focus it.

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../hooks/useSystemFeatures", () => ({ __esModule: true, default: () => ({ features: {} }) }));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
  useParams: () => ({}),
}));

describe("DatasourceForm: failed submit goes to the first invalid field", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockResolvedValue({ data: { data: [] } });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("focuses the empty Name field and scrolls it into view, without saving", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <DatasourceForm />
        </MemoryRouter>
      </ThemeProvider>
    );
    fireEvent.click(await screen.findByRole("button", { name: /add data source/i }));

    const name = await screen.findByRole("textbox", { name: /^name/i });
    await waitFor(() => expect(name).toHaveFocus());
    expect(name).toHaveAttribute("aria-invalid", "true");
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
    expect(apiClient.post).not.toHaveBeenCalled();
  });
});
