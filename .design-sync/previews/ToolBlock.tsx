import { ToolBlock } from "axios-web";

export const ToolUse = () => (
  <div style={{ width: 520, padding: 8 }}>
    <ToolBlock
      type="tool_use"
      toolName="axios-system__run_command"
      content={'{"command":"docker ps --format \'{{.Names}} {{.Status}}\'"}'}
    />
  </div>
);

export const ToolResult = () => (
  <div style={{ width: 520, padding: 8 }}>
    <ToolBlock
      type="tool_result"
      toolName="axios-system__run_command"
      content={
        "axiosd            Up 3 hours\nollama            Up 3 hours (healthy)\npostgres-dev      Up 41 minutes"
      }
    />
  </div>
);

export const CallAndResultPair = () => (
  <div style={{ display: "flex", flexDirection: "column", gap: 8, width: 520, padding: 8 }}>
    <ToolBlock
      type="tool_use"
      toolName="axios-fs__read_file"
      content={'{"path":"~/.axios/providers.json"}'}
    />
    <ToolBlock
      type="tool_result"
      toolName="axios-fs__read_file"
      content={'{"anthropic":{"api_key":"axsec1:...","default_model":"claude-sonnet-4-6"}}'}
    />
  </div>
);
