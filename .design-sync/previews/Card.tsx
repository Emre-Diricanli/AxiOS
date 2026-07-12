import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
  Badge,
  Button,
} from "axios-web";

export const ProviderCard = () => (
  <Card style={{ width: 340 }}>
    <CardHeader>
      <CardTitle>Anthropic</CardTitle>
      <CardDescription>anthropic.com — cloud provider</CardDescription>
    </CardHeader>
    <CardContent>
      <p style={{ fontSize: 12, fontFamily: "JetBrains Mono, monospace", opacity: 0.8 }}>
        claude-sonnet-4-6
      </p>
    </CardContent>
    <CardFooter style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <Button size="sm">Use</Button>
      <Badge variant="secondary">Connected</Badge>
    </CardFooter>
  </Card>
);

export const StatCard = () => (
  <Card style={{ width: 260 }}>
    <CardHeader>
      <CardTitle>Memory</CardTitle>
      <CardDescription>21.4 GB of 32 GB in use</CardDescription>
    </CardHeader>
    <CardContent>
      <div
        style={{
          height: 6,
          borderRadius: 3,
          background: "rgba(255,255,255,0.06)",
          overflow: "hidden",
        }}
      >
        <div style={{ width: "67%", height: "100%", background: "var(--color-primary)" }} />
      </div>
    </CardContent>
  </Card>
);
