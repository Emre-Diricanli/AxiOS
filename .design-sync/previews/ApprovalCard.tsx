import { ApprovalCard } from "axios-web";

// ApprovalCard's leading warning glyph is "⚠", which collides with the capture
// harness's error-boundary sentinel (cell textContent starting with "⚠").
// A hidden marker keeps textContent from starting with that glyph.
const Marker = () => <span style={{ display: "none" }}>approval-card preview</span>;

export const Pending = () => (
  <div style={{ width: 520, padding: 8 }}>
    <Marker />
    <ApprovalCard
      toolName="axios-system__run_command"
      params={'{"command":"systemctl restart axiosd"}'}
      status="pending"
      onRespond={() => {}}
    />
  </div>
);

export const Approved = () => (
  <div style={{ width: 520, padding: 8 }}>
    <Marker />
    <ApprovalCard
      toolName="axios-fs__write_file"
      params={'{"path":"~/.axios/hosts.json","content":"{\\"local\\":{\\"address\\":\\"127.0.0.1:3000\\"}}"}'}
      status="approved"
      onRespond={() => {}}
    />
  </div>
);

export const Denied = () => (
  <div style={{ width: 520, padding: 8 }}>
    <Marker />
    <ApprovalCard
      toolName="axios-system__run_command"
      params={'{"command":"rm -rf ~/.axios/sessions.json"}'}
      status="denied"
      onRespond={() => {}}
    />
  </div>
);
