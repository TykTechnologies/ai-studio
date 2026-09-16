import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../../../utils/testTheme";
import SelectAPI from "./SelectAPI";

const many = Array.from({ length: 1000 }, (_, i) => ({
  api_id: `api-${i}`,
  name: `API ${i}`,
  listen_path: `/api-${i}/`,
  active: i % 2 === 0,
}));

const renderList = (apis, props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <SelectAPI apis={apis} selectedAPI={null} onSelect={jest.fn()} loading={false} error="" {...props} />
    </ThemeProvider>,
  );

describe("SelectAPI", () => {
  it("caps a large list, keeps the search in view and narrows by search", () => {
    renderList(many);
    expect(screen.getAllByRole("button")).toHaveLength(200);
    expect(screen.getByTestId("api-overflow")).toHaveTextContent("first 200 of 1000");
    expect(screen.getByTestId("api-list")).toHaveStyle({ overflowY: "auto" });
    fireEvent.change(screen.getByTestId("api-search"), { target: { value: "/api-99" } });
    // api-99, api-990..999
    expect(screen.getAllByRole("button")).toHaveLength(11);
    expect(screen.queryByTestId("api-overflow")).not.toBeInTheDocument();
    fireEvent.change(screen.getByTestId("api-search"), { target: { value: "nothing here" } });
    expect(screen.getByText(/No API matches/)).toBeInTheDocument();
  });

  it("selects a row and marks inactive APIs", () => {
    const onSelect = jest.fn();
    renderList(many.slice(0, 2), { onSelect });
    expect(screen.getByText("inactive")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("api-api-0"));
    expect(onSelect).toHaveBeenCalledWith(many[0]);
  });
});
