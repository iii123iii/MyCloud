// Shared settings section header. Extracted from settings/page.tsx so the
// Developer section (and any future settings panels) render an identical title
// block without duplicating the markup.
export function SectionHeader({ title, description }: { title: string; description: string }) {
  return (
    <div className="space-y-0.5">
      <h2 className="text-lg font-semibold leading-tight tracking-tight">{title}</h2>
      <p className="text-sm text-muted-foreground">{description}</p>
    </div>
  );
}
