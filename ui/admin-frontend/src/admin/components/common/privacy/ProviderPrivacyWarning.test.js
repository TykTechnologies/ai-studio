import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import "@testing-library/jest-dom";
import PrivacyLevelInput from "./PrivacyLevelInput";
import ProviderPrivacyWarning from "./ProviderPrivacyWarning";

// #503: on the admin tool and data source forms (and the reviewer's final
// score), a privacy level above every active LLM provider is flagged, since
// nothing could then use the resource.

jest.mock("../../../utils/listAll", () => ({
  __esModule: true,
  listAll: jest.fn(),
}));
const { listAll } = require("../../../utils/listAll");

const llm = (name, privacy_score, active = true) => ({ id: name, attributes: { name, privacy_score, active } });
const providers = (...list) => listAll.mockResolvedValue({ data: { data: list } });

describe("ProviderPrivacyWarning", () => {
  it("warns above the highest active provider and names it", async () => {
    providers(llm("OpenAI", 50), llm("Claude", 40), llm("Off", 90, false));
    render(<ProviderPrivacyWarning score={60} />);
    const warning = await screen.findByTestId("provider-privacy-warning");
    expect(warning).toHaveTextContent("No active LLM provider can use this");
    expect(warning).toHaveTextContent("Internal · 50 (OpenAI)");
    expect(listAll).toHaveBeenCalledWith(expect.anything(), "/llms");
  });

  it("says nothing at or below the highest active provider", async () => {
    providers(llm("OpenAI", 50));
    render(<ProviderPrivacyWarning score={50} />);
    // Let the provider fetch settle before asserting absence.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(screen.queryByTestId("provider-privacy-warning")).not.toBeInTheDocument();
    expect(screen.queryByTestId("provider-privacy-hint")).not.toBeInTheDocument();
  });

  it("gives a neutral hint, not a warning, when there are no active providers", async () => {
    providers(llm("Off", 90, false));
    render(<ProviderPrivacyWarning score={100} />);
    expect(await screen.findByTestId("provider-privacy-hint")).toHaveTextContent("no active LLM providers yet");
    expect(screen.queryByTestId("provider-privacy-warning")).not.toBeInTheDocument();
  });

  it("stays silent when the providers cannot be read", async () => {
    listAll.mockRejectedValue(new Error("403"));
    render(<ProviderPrivacyWarning score={100} />);
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(screen.queryByTestId("provider-privacy-warning")).not.toBeInTheDocument();
    expect(screen.queryByTestId("provider-privacy-hint")).not.toBeInTheDocument();
  });
});

describe("PrivacyLevelInput checkProviders", () => {
  const Harness = ({ initial, checkProviders }) => {
    const [value, setValue] = React.useState(initial);
    return <PrivacyLevelInput value={value} onChange={setValue} checkProviders={checkProviders} />;
  };

  it("follows the score as it is edited", async () => {
    providers(llm("OpenAI", 50));
    render(<Harness initial={40} checkProviders />);
    const score = screen.getByRole("spinbutton", { name: /score/i });
    fireEvent.change(score, { target: { value: "80" } });
    expect(await screen.findByTestId("provider-privacy-warning")).toBeInTheDocument();
    fireEvent.change(score, { target: { value: "30" } });
    expect(screen.queryByTestId("provider-privacy-warning")).not.toBeInTheDocument();
  });

  it("does not look providers up unless asked (contributor and other forms)", () => {
    render(<Harness initial={100} />);
    expect(listAll).not.toHaveBeenCalled();
  });
});
