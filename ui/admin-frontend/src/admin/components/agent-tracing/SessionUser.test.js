import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import SessionUser from "./SessionUser";

const PSEUDONYM = "0123456789abcdef0123456789abcdef";

describe("SessionUser", () => {
  it("prefers the Studio user's name", () => {
    render(<SessionUser session={{ user_name: "Ada", end_user: PSEUDONYM, user_id: 3 }} masked />);
    expect(screen.getByText("Ada")).toBeInTheDocument();
    expect(screen.queryByTestId("end-user-pseudonym")).not.toBeInTheDocument();
  });

  it("shows the end user as sent when the viewer can read content", () => {
    render(<SessionUser session={{ end_user: "alice@example.com" }} masked={false} />);
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
  });

  it("labels the end user as a pseudonym when the viewer cannot read content", () => {
    render(<SessionUser session={{ end_user: PSEUDONYM }} masked />);
    expect(screen.getByTestId("end-user-pseudonym")).toHaveTextContent(`${PSEUDONYM} (pseudonym)`);
    expect(screen.getByLabelText(/your role sees end users as stable pseudonyms/)).toBeInTheDocument();
  });

  it("falls back to the Studio user id, then a dash", () => {
    const { rerender } = render(<SessionUser session={{ user_id: 3 }} masked />);
    expect(screen.getByText("User 3")).toBeInTheDocument();
    rerender(<SessionUser session={{}} masked />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
