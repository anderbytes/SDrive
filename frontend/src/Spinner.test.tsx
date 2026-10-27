import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { LoadingStatus, Spinner } from "./Spinner";

afterEach(cleanup);

test("loading status announces its text while the spinner remains decorative", () => {
  const view = render(<LoadingStatus large>Preparing your drives…</LoadingStatus>);
  const status = screen.getByRole("status");
  expect(status.textContent).toBe("Preparing your drives…");
  expect(status.querySelector(".spinner-large")?.getAttribute("aria-hidden")).toBe("true");
  view.rerender(<Spinner />);
  expect(screen.queryByRole("status")).toBeNull();
  expect(view.container.querySelector(".spinner-large")).toBeNull();
});
