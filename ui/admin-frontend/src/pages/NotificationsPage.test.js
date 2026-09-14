import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import NotificationsPage from "./NotificationsPage";

jest.mock("../admin/components/notifications/NotificationList", () => () => <div data-testid="notification-list" />);

describe("NotificationsPage", () => {
  it("renders the notification list", () => {
    render(<NotificationsPage />);
    expect(screen.getByTestId("notification-list")).toBeInTheDocument();
  });
});
