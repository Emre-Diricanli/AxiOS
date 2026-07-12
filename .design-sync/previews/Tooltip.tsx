import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider, Button } from "axios-web";

export const OpenTop = () => (
  <TooltipProvider>
    <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "56px 24px 16px" }}>
      <Tooltip open>
        <TooltipTrigger render={<Button variant="outline">claude-sonnet-4</Button>} />
        <TooltipContent>Default model for this session</TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
);

export const OpenBottom = () => (
  <TooltipProvider>
    <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "16px 24px 56px" }}>
      <Tooltip open>
        <TooltipTrigger render={<Button variant="ghost">axios-fs</Button>} />
        <TooltipContent side="bottom">MCP server · filesystem tools</TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
);
