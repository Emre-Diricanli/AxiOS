import { Badge } from "axios-web";

export const Variants = () => (
  <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap", padding: 8 }}>
    <Badge>claude-sonnet-4</Badge>
    <Badge variant="secondary">ollama</Badge>
    <Badge variant="destructive">rate limited</Badge>
    <Badge variant="outline">gpt-4o</Badge>
    <Badge variant="ghost">local</Badge>
  </div>
);

export const ProviderStatus = () => (
  <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap", padding: 8 }}>
    <Badge variant="secondary">anthropic</Badge>
    <Badge>connected</Badge>
    <Badge variant="outline">3 models</Badge>
    <Badge variant="destructive">key expired</Badge>
  </div>
);
