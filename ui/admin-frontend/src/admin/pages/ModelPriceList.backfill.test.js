import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import ModelPriceList from "./ModelPriceList";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({
    can: () => true,
    canAny: () => true,
    canAll: () => true,
    isFullAdmin: true,
    hasAdminAccess: true,
    rbacEnabled: false,
  }),
}));

const price = (unpriced) => ({
  type: "model-prices",
  id: "7",
  attributes: { model_name: "gemini-2.5-flash", vendor: "google_ai", cpt: 0.0000025, cpit: 0.0000003, cache_write_pt: 0, cache_read_pt: 0, currency: "USD" },
  meta: { unpriced_rows: unpriced },
});

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/model-prices"]}>
        <ModelPriceList />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("ModelPriceList backfill", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows each price's unpriced requests and backfills them over a date range", async () => {
    apiClient.get
      .mockResolvedValueOnce({ data: { data: [price(14320)] }, headers: { "x-total-count": "1", "x-total-pages": "1" } })
      .mockResolvedValue({ data: { data: [price(0)] }, headers: { "x-total-count": "1", "x-total-pages": "1" } });
    apiClient.post.mockResolvedValue({ data: { data: { rows_priced: 14320 } } });
    renderPage();

    expect(await screen.findByText((14320).toLocaleString())).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("row-actions"));
    fireEvent.click(await screen.findByText("Backfill unpriced history", { selector: "li, li *" }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent(/were recorded at no cost/);

    fireEvent.change(within(dialog).getByLabelText("From"), { target: { value: "2026-05-01" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Backfill" }));

    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/model-prices/7/backfill", { from: "2026-05-01" }));
    expect(await screen.findByText(/Priced 14,320 unpriced requests/)).toBeInTheDocument();
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledTimes(2));
  });
});
