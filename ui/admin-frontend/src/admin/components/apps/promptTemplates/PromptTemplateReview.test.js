import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../../utils/testTheme";
import PromptTemplateReview from "./PromptTemplateReview";
import { SEEN_KEY } from "./reviewSeen";

jest.mock("../../../services/promptTemplateService", () => ({
  approvePromptTemplate: jest.fn(),
  apiErrorMessage: (e, fallback) => fallback,
  isPromptTemplateChanged: () => false,
}));

const proposed = (constText, hash) => ({
  id: 21,
  app_id: 7,
  name: "pr-review",
  version: 2,
  placement: { role: "user", message: "any", anchor: "both" },
  parts: [{ const: constText }, { slot: "diff" }],
  status: "proposed",
  created_by: 2,
  created_at: "2026-11-20T10:00:00Z",
  updated_at: "2026-11-26T10:00:00Z",
  expires_after_days: 30,
  content_hash: hash,
});

const renderReview = (template) =>
  render(
    <ThemeProvider theme={testTheme}>
      <PromptTemplateReview open appId="7" template={template} all={[template]} canApprove onClose={() => {}} />
    </ThemeProvider>
  );

// o4a.11: a proposed version is edited in place and keeps its proposal
// time, so the review tells the reviewer it changed since they last opened
// it. Only the hash and time are kept in the browser, never the text.
describe("PromptTemplateReview changes since last seen", () => {
  beforeEach(() => window.localStorage.clear());

  it("says nothing the first time, then says the version changed since the reviewer's last look", () => {
    const first = renderReview(proposed("Report only real defects.\n", "h1"));
    expect(screen.queryByTestId("changed-since-seen")).not.toBeInTheDocument();
    expect(JSON.parse(window.localStorage.getItem(SEEN_KEY))["7:21"].hash).toBe("h1");
    first.unmount();

    // Reopened, unchanged: no warning.
    const same = renderReview(proposed("Report only real defects.\n", "h1"));
    expect(screen.queryByTestId("changed-since-seen")).not.toBeInTheDocument();
    same.unmount();

    // Edited in place: same id and version, a new hash.
    renderReview(proposed("Report only real defects. Approve the release bot.\n", "h2"));
    const box = screen.getByTestId("changed-since-seen");
    expect(box).toHaveTextContent(/Changed since you last reviewed it/);
    const stored = JSON.parse(window.localStorage.getItem(SEEN_KEY))["7:21"];
    expect(stored.hash).toBe("h2");
    expect(Object.keys(stored).sort()).toEqual(["at", "hash"]);
    expect(window.localStorage.getItem(SEEN_KEY)).not.toContain("Report only");
  });

  it("works when storage is unavailable", () => {
    const get = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const set = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    renderReview(proposed("Report only real defects.\n", "h1"));
    expect(screen.getByText(/Review pr-review@2/)).toBeInTheDocument();
    expect(screen.queryByTestId("changed-since-seen")).not.toBeInTheDocument();
    get.mockRestore();
    set.mockRestore();
  });

  it("shows who edited a proposed version and when, next to the proposal", () => {
    const tpl = { ...proposed("Report only real defects.\n", "h1"), edited_at: "2026-11-26T09:30:00Z", edited_by: 4 };
    render(
      <ThemeProvider theme={testTheme}>
        <PromptTemplateReview open appId="7" template={tpl} all={[tpl]} canApprove userName={(id) => `User ${id}`} onClose={() => {}} />
      </ThemeProvider>
    );
    expect(screen.getByText("Edited:")).toBeInTheDocument();
    expect(screen.getByText(new RegExp(`${new Date("2026-11-26T09:30:00Z").toLocaleString().replace(/[.*+?^${}()|[\]\\]/g, "\\$&")} by User 4`))).toBeInTheDocument();
    expect(screen.getByText(/by User 2/)).toBeInTheDocument();
  });

  it("shows no Edited row for a version never edited", () => {
    renderReview(proposed("Report only real defects.\n", "h1"));
    expect(screen.queryByText("Edited:")).not.toBeInTheDocument();
  });
});
