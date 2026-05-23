// Tests for BreadcrumbNav. Covers: empty path → Home only, navigation
// callback on click + Enter + Space, copy-to-clipboard, current-page aria
// markup, and the keyboard activation patterns the component re-implements
// (BreadcrumbLink is `<a>` without href, so it needs explicit Enter/Space
// handling for keyboard users).

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BreadcrumbNav } from "./BreadcrumbNav";

// Userland event without @testing-library — we use the global default.
// userEvent.setup() returns a controlled user for ergonomic interactions.

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const folder = (id: string, name: string) =>
  ({ id, name, parent_id: null, created_at: "", updated_at: "" } as any);

describe("<BreadcrumbNav />", () => {
  it("renders empty path with current Home only", () => {
    render(<BreadcrumbNav path={[]} onNavigate={vi.fn()} />);
    expect(screen.getByText(/my files/i)).toBeInTheDocument();
  });

  it("renders a crumb per path segment", () => {
    render(
      <BreadcrumbNav
        path={[folder("p1", "Photos"), folder("p2", "2026"), folder("p3", "May")]}
        onNavigate={vi.fn()}
      />,
    );
    expect(screen.getByText("Photos")).toBeInTheDocument();
    expect(screen.getByText("2026")).toBeInTheDocument();
    expect(screen.getByText("May")).toBeInTheDocument();
  });

  it("clicking Home calls onNavigate(-1)", async () => {
    const onNav = vi.fn();
    const user = userEvent.setup();
    render(
      <BreadcrumbNav path={[folder("p", "Docs")]} onNavigate={onNav} />,
    );
    // BreadcrumbLink renders with role="button" + aria-label="My Files".
    const homeBtn = screen.getByRole("button", { name: /my files/i });
    await user.click(homeBtn);
    expect(onNav).toHaveBeenCalledWith(-1);
  });

  it("clicking middle crumb fires onNavigate with its index", async () => {
    const onNav = vi.fn();
    const user = userEvent.setup();
    render(
      <BreadcrumbNav
        path={[folder("p1", "A"), folder("p2", "B"), folder("p3", "C")]}
        onNavigate={onNav}
      />,
    );
    // Middle crumb "A" is rendered as a clickable BreadcrumbLink role=button.
    const aBtn = screen.getByRole("button", { name: "A" });
    await user.click(aBtn);
    expect(onNav).toHaveBeenCalledWith(0);
  });

  it("Enter on focused crumb activates", async () => {
    const onNav = vi.fn();
    const user = userEvent.setup();
    render(
      <BreadcrumbNav
        path={[folder("p1", "A"), folder("p2", "B")]}
        onNavigate={onNav}
      />,
    );
    const aBtn = screen.getByRole("button", { name: "A" });
    aBtn.focus();
    await user.keyboard("{Enter}");
    expect(onNav).toHaveBeenCalledWith(0);
  });

  it("Space on focused crumb activates", async () => {
    const onNav = vi.fn();
    const user = userEvent.setup();
    render(
      <BreadcrumbNav
        path={[folder("p1", "A"), folder("p2", "B")]}
        onNavigate={onNav}
      />,
    );
    const aBtn = screen.getByRole("button", { name: "A" });
    aBtn.focus();
    await user.keyboard(" ");
    expect(onNav).toHaveBeenCalledWith(0);
  });

  it("last segment renders as current page (aria-current)", () => {
    render(
      <BreadcrumbNav
        path={[folder("p1", "Photos"), folder("p2", "2026")]}
        onNavigate={vi.fn()}
      />,
    );
    // The current crumb is rendered with BreadcrumbPage which gets
    // aria-current="page".
    const current = screen.getByText("2026");
    expect(current.closest("[aria-current=page]")).not.toBeNull();
  });

  it("copy button is rendered when path is non-empty", () => {
    render(
      <BreadcrumbNav
        path={[folder("p1", "Photos"), folder("p2", "2026")]}
        onNavigate={vi.fn()}
      />,
    );
    // The copy button has aria-label="Copy folder path". We assert it
    // exists but don't drive the clipboard write — jsdom doesn't expose a
    // testable navigator.clipboard by default, and an env-specific stub
    // hasn't been worth the test value.
    expect(screen.getByRole("button", { name: /copy folder path/i })).toBeInTheDocument();
  });

  it("copy button not rendered for empty path", () => {
    render(<BreadcrumbNav path={[]} onNavigate={vi.fn()} />);
    expect(screen.queryByRole("button", { name: /copy folder path/i })).toBeNull();
  });
});
