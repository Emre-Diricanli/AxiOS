import { InputBar } from "axios-web";

export const Default = () => (
  <div style={{ width: 520, padding: 8 }}>
    <InputBar
      onSend={() => {}}
      modelName="claude-sonnet-4-6"
      onToggleCodeMode={() => {}}
    />
  </div>
);

export const CodeModeActive = () => (
  <div style={{ width: 520, padding: 8 }}>
    <InputBar
      onSend={() => {}}
      modelName="claude-sonnet-4-6"
      codeMode
      codeActive
      codeDir="~/axios-workspace"
      onToggleCodeMode={() => {}}
      onCodeDirChange={() => {}}
    />
  </div>
);

export const Streaming = () => (
  <div style={{ width: 520, padding: 8 }}>
    <InputBar
      onSend={() => {}}
      modelName="claude-sonnet-4-6"
      codeActive
      streaming
      onToggleCodeMode={() => {}}
      onAbort={() => {}}
    />
  </div>
);
