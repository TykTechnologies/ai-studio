import React from "react";
import { render, screen } from "@testing-library/react";
import ListTruncatedToaster from "./ListTruncatedToaster";
import { listAll, LIST_ALL_PAGE_SIZE, LIST_ALL_MAX_ROWS } from "../../admin/utils/listAll";

const clientWith = (totalPages) => ({
  get: jest.fn(async () => ({
    data: { data: Array.from({ length: LIST_ALL_PAGE_SIZE }, (_, i) => ({ id: String(i) })) },
    headers: { "x-total-pages": String(totalPages) },
  })),
});

describe("ListTruncatedToaster", () => {
  it("stays quiet for a list that fits", async () => {
    render(<ListTruncatedToaster />);
    await listAll(clientWith(2), "/groups");
    expect(screen.queryByTestId("list-truncated-toast")).toBeNull();
  });

  it("tells the user when a list was capped", async () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    render(<ListTruncatedToaster />);
    await listAll(clientWith(LIST_ALL_MAX_ROWS / LIST_ALL_PAGE_SIZE + 5), "/groups");
    const toast = await screen.findByTestId("list-truncated-toast");
    expect(toast).toHaveTextContent(`only the first ${LIST_ALL_MAX_ROWS} are shown`);
    warn.mockRestore();
  });
});
