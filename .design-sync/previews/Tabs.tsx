import { Tabs, TabsList, TabsTrigger, TabsContent } from "axios-web";

export const Default = () => (
  <div style={{ display: "flex", padding: 8, maxWidth: 420 }}>
    {/* flex-col: root's data-horizontal:flex-col never matches (base-ui emits data-orientation) */}
    <Tabs defaultValue="providers" className="flex-col">
      <TabsList>
        <TabsTrigger value="providers">Providers</TabsTrigger>
        <TabsTrigger value="models">Models</TabsTrigger>
        <TabsTrigger value="hosts">Hosts</TabsTrigger>
      </TabsList>
      <TabsContent value="providers">
        anthropic and ollama are connected. 5 models available.
      </TabsContent>
      <TabsContent value="models">Model list</TabsContent>
      <TabsContent value="hosts">Host list</TabsContent>
    </Tabs>
  </div>
);

export const LineVariant = () => (
  <div style={{ display: "flex", padding: 8, maxWidth: 420 }}>
    <Tabs defaultValue="system" className="flex-col">
      <TabsList variant="line">
        <TabsTrigger value="system">System</TabsTrigger>
        <TabsTrigger value="containers">Containers</TabsTrigger>
        <TabsTrigger value="logs">Logs</TabsTrigger>
      </TabsList>
      <TabsContent value="system">
        CPU 12% · RAM 4.2 GB · 3 containers running
      </TabsContent>
      <TabsContent value="containers">Container list</TabsContent>
      <TabsContent value="logs">Log stream</TabsContent>
    </Tabs>
  </div>
);
