import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Link } from "react-router-dom";
import QuotaNagBar, { PENDING_KEY } from "./QuotaNagBar";

jest.mock("../../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("../../../components/common/Icon", () => ({
  __esModule: true,
  default: ({ name }) => <span>{name}</span>,
}));

const pubClient = require("../../utils/pubClient").default;

const appsNotice = {
  key: "quota:limit-a:10",
  label: "Apps",
  used: 9,
  max: 10,
  unlimited: false,
  message: "Your organisation is using 9 of its 10 apps.",
};
const quotaNotice = {
  key: "quota:limit-b:2",
  label: "Your apps",
  used: 1,
  max: 2,
  unlimited: false,
  message: "You are using 1 of your quota of 2 apps.",
};

const renderBar = () =>
  render(
    <MemoryRouter>
      <QuotaNagBar />
    </MemoryRouter>,
  );

describe("QuotaNagBar", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    pubClient.post.mockResolvedValue({});
  });

  it("renders every notice the API returns", async () => {
    pubClient.get.mockResolvedValue({ data: { data: [appsNotice, quotaNotice] } });
    renderBar();
    expect(await screen.findByText(appsNotice.message)).toBeInTheDocument();
    expect(screen.getByText(quotaNotice.message)).toBeInTheDocument();
    expect(screen.getByText("Apps")).toBeInTheDocument();
    expect(pubClient.get).toHaveBeenCalledWith("/common/notices");
  });

  it("dismisses a notice through the API and hides it", async () => {
    pubClient.get.mockResolvedValue({ data: { data: [appsNotice, quotaNotice] } });
    renderBar();
    const banner = (await screen.findByText(appsNotice.message)).closest("[data-testid=quota-nag-bar] > *");
    fireEvent.click(within(banner).getByRole("button"));

    await waitFor(() =>
      expect(pubClient.post).toHaveBeenCalledWith("/common/notices/quota%3Alimit-a%3A10/dismiss"),
    );
    expect(screen.queryByText(appsNotice.message)).not.toBeInTheDocument();
    expect(screen.getByText(quotaNotice.message)).toBeInTheDocument();
  });

  it("renders nothing without notices or when the request fails", async () => {
    pubClient.get.mockResolvedValue({ data: { data: [] } });
    const { container, unmount } = renderBar();
    await waitFor(() => expect(pubClient.get).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
    unmount();

    pubClient.get.mockRejectedValue(new Error("network"));
    const second = renderBar();
    await waitFor(() => expect(pubClient.get).toHaveBeenCalledTimes(2));
    expect(second.container).toBeEmptyDOMElement();
  });

  // tkg.12: leaving the page right after the click aborted the dismissal, and
  // the next page showed the banner again.
  it("keeps a dismissal whose request never completed, and sends it again", async () => {
    pubClient.get.mockResolvedValue({ data: { data: [appsNotice] } });
    pubClient.post.mockReturnValue(new Promise(() => {})); // aborted by the navigation
    const first = renderBar();
    fireEvent.click(within(await screen.findByTestId("quota-nag-bar")).getByRole("button"));
    expect(screen.queryByText(appsNotice.message)).not.toBeInTheDocument();
    first.unmount();

    // The next page load: the server never stored it and still sends it.
    pubClient.post.mockResolvedValue({});
    const second = renderBar();
    await waitFor(() => expect(pubClient.get).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(pubClient.post).toHaveBeenCalledTimes(2));
    expect(pubClient.post).toHaveBeenLastCalledWith("/common/notices/quota%3Alimit-a%3A10/dismiss");
    expect(second.container).toBeEmptyDOMElement();
    await waitFor(() => expect(window.localStorage.getItem(PENDING_KEY)).toBeNull());
  });

  it("does not bring a dismissed notice back when the next page's request returns it", async () => {
    pubClient.get.mockResolvedValue({ data: { data: [appsNotice, quotaNotice] } });
    pubClient.post.mockReturnValue(new Promise(() => {})); // still in flight
    render(
      <MemoryRouter initialEntries={["/admin/a"]}>
        <Link to="/admin/b">next</Link>
        <QuotaNagBar />
      </MemoryRouter>,
    );
    const banner = (await screen.findByText(appsNotice.message)).closest("[data-testid=quota-nag-bar] > *");
    fireEvent.click(within(banner).getByRole("button"));
    fireEvent.click(screen.getByText("next"));

    await waitFor(() => expect(pubClient.get).toHaveBeenCalledTimes(2));
    expect(await screen.findByText(quotaNotice.message)).toBeInTheDocument();
    expect(screen.queryByText(appsNotice.message)).not.toBeInTheDocument();
  });

  it("forgets a pending dismissal the server refuses as no longer current", async () => {
    window.localStorage.setItem(PENDING_KEY, JSON.stringify([appsNotice.key]));
    pubClient.get.mockResolvedValue({ data: { data: [appsNotice] } });
    pubClient.post.mockRejectedValue({ response: { status: 400 } });
    renderBar();
    await waitFor(() => expect(pubClient.post).toHaveBeenCalled());
    await waitFor(() => expect(window.localStorage.getItem(PENDING_KEY)).toBeNull());
  });
});
