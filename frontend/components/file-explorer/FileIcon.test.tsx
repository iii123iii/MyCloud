// Tests for FileIcon. Pure-render component — verify it picks the right
// icon + colour by mime + isFolder, exposes accessible labels in
// interactive mode, and renders the thumbnail-with-fallback path.

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FileIcon } from "./FileIcon";

// Helper: render a FileIcon and assert it picked a particular icon by
// checking the rendered SVG's class for the expected color.

describe("<FileIcon />", () => {
  it("folder shows yellow folder icon", () => {
    const { container } = render(<FileIcon isFolder size="md" />);
    const root = container.querySelector(".text-yellow-500");
    expect(root).not.toBeNull();
  });

  it("image MIME shows emerald image icon", () => {
    const { container } = render(<FileIcon mime="image/png" size="md" />);
    expect(container.querySelector(".text-emerald-500")).not.toBeNull();
  });

  it("video MIME shows violet video icon", () => {
    const { container } = render(<FileIcon mime="video/mp4" size="md" />);
    expect(container.querySelector(".text-violet-500")).not.toBeNull();
  });

  it("audio MIME shows pink audio icon", () => {
    const { container } = render(<FileIcon mime="audio/mpeg" size="md" />);
    expect(container.querySelector(".text-pink-500")).not.toBeNull();
  });

  it("PDF MIME shows red PDF icon", () => {
    const { container } = render(<FileIcon mime="application/pdf" size="md" />);
    expect(container.querySelector(".text-red-500")).not.toBeNull();
  });

  it("text MIME shows sky text icon", () => {
    const { container } = render(<FileIcon mime="text/plain" size="md" />);
    expect(container.querySelector(".text-sky-500")).not.toBeNull();
  });

  it("zip MIME shows amber archive icon", () => {
    const { container } = render(<FileIcon mime="application/zip" size="md" />);
    expect(container.querySelector(".text-amber-500")).not.toBeNull();
  });

  it("excel MIME shows green spreadsheet icon", () => {
    const { container } = render(
      <FileIcon
        mime="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        size="md"
      />,
    );
    expect(container.querySelector(".text-green-600")).not.toBeNull();
  });

  it("unknown MIME falls back to muted-foreground", () => {
    const { container } = render(<FileIcon mime="application/whatever" size="md" />);
    expect(container.querySelector(".text-muted-foreground")).not.toBeNull();
  });

  it("interactive mode renders a button with aria-label", () => {
    render(
      <FileIcon
        mime="image/png"
        fileName="vacation.png"
        size="md"
        interactive
        onClick={() => {}}
      />,
    );
    expect(
      screen.getByRole("button", { name: /vacation\.png.*image/i }),
    ).toBeInTheDocument();
  });

  it("interactive click fires onClick", async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(
      <FileIcon
        mime="image/png"
        fileName="x.png"
        size="md"
        interactive
        onClick={onClick}
      />,
    );
    await user.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("interactive without fileName labels by type", () => {
    render(<FileIcon mime="image/png" size="md" interactive onClick={() => {}} />);
    expect(screen.getByRole("button", { name: /image/i })).toBeInTheDocument();
  });

  it("size 'xl' applies xl box classes", () => {
    const { container } = render(<FileIcon mime="image/png" size="xl" />);
    const box = container.querySelector(".h-20.w-20");
    expect(box).not.toBeNull();
  });

  it("bare (no size) renders just the svg without box wrapper", () => {
    const { container } = render(<FileIcon mime="image/png" />);
    // No BOX_SIZES rounded class on the wrapper since size was omitted.
    expect(container.querySelector(".rounded-md")).toBeNull();
    expect(container.querySelector(".rounded-lg")).toBeNull();
  });

  it("thumbnail URL triggers img element rendering", () => {
    const { container } = render(
      <FileIcon
        mime="image/png"
        thumbnailUrl="/api/v2/files/x/thumb"
        size="md"
        fileName="x.png"
      />,
    );
    expect(container.querySelector("img")).not.toBeNull();
  });
});
