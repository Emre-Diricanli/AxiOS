import { DiffBlock } from "axios-web";

const serverPatch = `@@ -14,7 +14,9 @@ func (s *Server) routes() {
 	mux.HandleFunc("/api/chat", s.handleChat)
 	mux.HandleFunc("/api/models", s.handleModels)
-	mux.HandleFunc("/api/tasks", s.handleTasks)
+	mux.HandleFunc("/api/tasks", s.requireAuth(s.handleTasks))
+	mux.HandleFunc("/api/tasks/", s.requireAuth(s.handleTaskDetail))
+	mux.HandleFunc("/api/health", s.handleHealth)
 }`;

const configPatch = `@@ -3,6 +3,7 @@ server:
   listen: 127.0.0.1:3000
 providers:
   default: anthropic
+  fallback: ollama
 mcp:
   socket_dir: /tmp/axios-mcp`;

const routerPatch = `@@ -22,8 +22,6 @@ func (r *Router) Pick(model string) Provider {
 	if p, ok := r.byModel[model]; ok {
 		return p
 	}
-	// TODO: remove legacy fallback
-	return r.legacy
+	return r.defaultProvider
 }`;

export const SingleFile = () => (
  <div style={{ width: 520, padding: 8 }}>
    <DiffBlock
      files={[
        {
          file: "internal/axiosd/server.go",
          patch: serverPatch,
          additions: 3,
          deletions: 1,
          status: "modified",
        },
      ]}
    />
  </div>
);

export const MultipleFiles = () => (
  <div style={{ width: 520, padding: 8 }}>
    <DiffBlock
      files={[
        {
          file: "configs/axiosd.yaml",
          patch: configPatch,
          additions: 1,
          deletions: 0,
          status: "modified",
        },
        {
          file: "internal/axiosd/router.go",
          patch: routerPatch,
          additions: 1,
          deletions: 3,
          status: "modified",
        },
        {
          file: "internal/axiosd/legacy_router.go",
          patch: "",
          additions: 0,
          deletions: 84,
          status: "deleted",
        },
      ]}
    />
  </div>
);
