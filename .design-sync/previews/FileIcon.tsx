import { FileIcon } from "axios-web";

export const FileTypes = () => (
  <div style={{ display: "flex", gap: 16, alignItems: "center", flexWrap: "wrap", padding: 8 }}>
    <FileIcon name="main.go" isDir={false} />
    <FileIcon name="index.tsx" isDir={false} />
    <FileIcon name="README.md" isDir={false} />
    <FileIcon name="photo.png" isDir={false} />
    <FileIcon name="axiosd.yaml" isDir={false} />
    <FileIcon name="backup.tar" isDir={false} />
    <FileIcon name="projects" isDir={true} />
    <FileIcon name="Downloads" isDir={true} />
  </div>
);

export const Sizes = () => (
  <div style={{ display: "flex", gap: 16, alignItems: "center", padding: 8 }}>
    <FileIcon name="server.go" isDir={false} size="sm" />
    <FileIcon name="server.go" isDir={false} size="md" />
    <FileIcon name="server.go" isDir={false} size="lg" />
    <FileIcon name="configs" isDir={true} size="sm" />
    <FileIcon name="configs" isDir={true} size="md" />
    <FileIcon name="configs" isDir={true} size="lg" />
  </div>
);
