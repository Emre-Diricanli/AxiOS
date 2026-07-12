import { Button } from "axios-web";

export const Variants = () => (
  <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap", padding: 8 }}>
    <Button>Deploy model</Button>
    <Button variant="secondary">Secondary</Button>
    <Button variant="outline">Outline</Button>
    <Button variant="ghost">Ghost</Button>
    <Button variant="destructive">Remove key</Button>
    <Button variant="link">Learn more</Button>
  </div>
);

export const Sizes = () => (
  <div style={{ display: "flex", gap: 12, alignItems: "center", padding: 8 }}>
    <Button size="xs">Extra small</Button>
    <Button size="sm">Small</Button>
    <Button size="default">Default</Button>
    <Button size="lg">Large</Button>
  </div>
);

export const Disabled = () => (
  <div style={{ display: "flex", gap: 12, alignItems: "center", padding: 8 }}>
    <Button disabled>Connecting…</Button>
    <Button variant="destructive" disabled>
      Remove key
    </Button>
  </div>
);
