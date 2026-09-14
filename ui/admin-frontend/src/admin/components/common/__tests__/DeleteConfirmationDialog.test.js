import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../../utils/testTheme";
import DeleteConfirmationDialog from "../DeleteConfirmationDialog";
import apiClient from "../../../utils/apiClient";

jest.mock("../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../../../../components/common/Icon", () => (props) => <div data-testid="mock-icon">{props.name}</div>);

const renderDialog = (props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <DeleteConfirmationDialog
        open
        resourcePath="llms"
        objectLabel="LLM"
        item={{ id: "3", name: "OpenAI Prod" }}
        consequence="Deleting it removes it from all of them; apps that only have this LLM will stop working."
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
        {...props}
      />
    </ThemeProvider>,
  );

describe("DeleteConfirmationDialog", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("titles the dialog after the object and lists its dependents", async () => {
    apiClient.get.mockResolvedValue({
      data: {
        data: {
          type: "dependents",
          id: "3",
          attributes: {
            apps: [{ id: 1, name: "Billing Copilot" }, { id: 2, name: "Quickstart App" }],
            catalogues: [{ id: 9, name: "Platform LLMs" }],
            llms: [], tools: [], datasources: [], agents: [], model_routers: [],
            total: 3,
          },
        },
      },
    });
    renderDialog();
    expect(screen.getByText("Delete OpenAI Prod?")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/llms/3/dependents");
    await waitFor(() =>
      expect(
        screen.getByText(
          "Used by 2 apps (Billing Copilot, Quickstart App) and 1 catalog (Platform LLMs). Deleting it removes it from all of them; apps that only have this LLM will stop working.",
        ),
      ).toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
  });

  it("says nothing references the object when there are no dependents", async () => {
    apiClient.get.mockResolvedValue({
      data: { data: { attributes: { apps: [], catalogues: [], llms: [], tools: [], datasources: [], agents: [], model_routers: [], total: 0 } } },
    });
    renderDialog({ objectLabel: "secret", item: { id: "4", name: "OPENAI_KEY" } });
    await waitFor(() => expect(screen.getByText("Nothing references this secret.")).toBeInTheDocument());
  });

  it("falls back to a generic consequence when the dependents request fails", async () => {
    jest.spyOn(console, "error").mockImplementation(() => {});
    apiClient.get.mockRejectedValue(new Error("404"));
    renderDialog({ objectLabel: "tool", item: { id: "8", name: "Weather" } });
    await waitFor(() =>
      expect(screen.getByText("Deleting this tool removes it from everything that references it.")).toBeInTheDocument(),
    );
    console.error.mockRestore();
  });

  it("calls onConfirm on Delete and onCancel on Cancel", async () => {
    apiClient.get.mockResolvedValue({ data: { data: { attributes: { total: 0 } } } });
    const onConfirm = jest.fn();
    const onCancel = jest.fn();
    renderDialog({ onConfirm, onCancel });
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });
});
