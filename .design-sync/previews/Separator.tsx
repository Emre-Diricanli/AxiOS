import { Separator } from "axios-web";

export const Horizontal = () => (
  <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 8, maxWidth: 360 }}>
    <div style={{ fontSize: 13 }}>Providers</div>
    {/* explicit h-px: base-ui emits data-orientation, app CSS targets [data-horizontal] */}
    <Separator className="h-px w-full" />
    <div style={{ fontSize: 13, opacity: 0.7 }}>anthropic — 3 models</div>
    <div style={{ fontSize: 13, opacity: 0.7 }}>ollama — 2 models</div>
  </div>
);

export const Vertical = () => (
  <div style={{ display: "flex", gap: 12, alignItems: "center", padding: 8, height: 40 }}>
    <span style={{ fontSize: 13 }}>CPU 12%</span>
    <Separator orientation="vertical" style={{ width: 1, height: 16 }} />
    <span style={{ fontSize: 13 }}>RAM 4.2 GB</span>
    <Separator orientation="vertical" style={{ width: 1, height: 16 }} />
    <span style={{ fontSize: 13 }}>Disk 118 GB free</span>
  </div>
);
