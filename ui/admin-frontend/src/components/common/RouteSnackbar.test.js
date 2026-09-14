import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter, Routes, Route, useLocation, useNavigate } from "react-router-dom";
import RouteSnackbar from "./RouteSnackbar";

const StateProbe = () => {
  const location = useLocation();
  return <div data-testid="state">{JSON.stringify(location.state)}</div>;
};

const Sender = () => {
  const navigate = useNavigate();
  return (
    <button
      onClick={() =>
        navigate("/admin/llms", {
          state: { snackbar: { message: "LLM created successfully", severity: "success" } },
        })
      }
    >
      save
    </button>
  );
};

const renderAt = (entries) =>
  render(
    <MemoryRouter initialEntries={entries}>
      <RouteSnackbar />
      <Routes>
        <Route path="/admin/llms/new" element={<Sender />} />
        <Route path="/admin/llms" element={<StateProbe />} />
      </Routes>
    </MemoryRouter>,
  );

describe("RouteSnackbar", () => {
  it("shows the message handed over in router state and clears it from history", async () => {
    renderAt([
      { pathname: "/admin/llms", state: { snackbar: { message: "Saved", severity: "success" }, keep: 1 } },
    ]);
    expect(await screen.findByText("Saved")).toBeInTheDocument();
    // The snackbar key is consumed; other state survives.
    await waitFor(() => expect(screen.getByTestId("state")).toHaveTextContent('{"keep":1}'));
  });

  it("renders nothing without a snackbar in state", () => {
    renderAt(["/admin/llms"]);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows the toast on the destination after an immediate navigate", async () => {
    renderAt(["/admin/llms/new"]);
    fireEvent.click(screen.getByText("save"));
    expect(await screen.findByText("LLM created successfully")).toBeInTheDocument();
    // Closing removes it.
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    await waitFor(() => expect(screen.queryByText("LLM created successfully")).not.toBeInTheDocument());
  });
});
