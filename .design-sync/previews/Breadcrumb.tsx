import { Breadcrumb } from "axios-web";

export const Canonical = () => (
  <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-start", padding: 8, maxWidth: 420 }}>
    <Breadcrumb path="/home/emre/projects/axios" onNavigate={() => {}} />
  </div>
);

export const Root = () => (
  <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-start", padding: 8, maxWidth: 420 }}>
    <Breadcrumb path="/" onNavigate={() => {}} />
  </div>
);

export const DeepPath = () => (
  <div style={{ display: "flex", flexDirection: "column", padding: 8, maxWidth: 380 }}>
    <Breadcrumb
      path="/home/emre/projects/axios/web/src/components/Files"
      onNavigate={() => {}}
    />
  </div>
);
