import { useState, useEffect } from "react";
import { Shell } from "@/components/Layout/Shell";
import { SetupWizard } from "@/components/Setup/SetupWizard";
import { ToastContainer } from "@/components/Layout/ToastContainer";

type SetupState = "loading" | "setup" | "ready";

export default function App() {
  const [state, setState] = useState<SetupState>("loading");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);

    // ?setup — force show wizard (for testing/re-running without deleting data)
    if (params.has("setup")) {
      setState("setup");
      return;
    }
    // ?skip-setup — skip wizard even if not completed (dev convenience)
    if (params.has("skip-setup")) {
      setState("ready");
      return;
    }

    fetch("/api/setup/status")
      .then((res) => (res.ok ? res.json() : { completed: true }))
      .then((data: { completed: boolean }) => {
        setState(data.completed ? "ready" : "setup");
      })
      .catch(() => {
        setState("ready");
      });
  }, []);

  if (state === "loading") {
    return (
      <div className="h-screen flex items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-4">
          <div className="relative w-10 h-10">
            <div className="absolute inset-0 rounded-full border-2 border-primary/20" />
            <div className="absolute inset-0 rounded-full border-2 border-transparent border-t-primary animate-spin" />
          </div>
          <p className="text-xs text-muted-foreground">Starting AxiOS...</p>
        </div>
      </div>
    );
  }

  if (state === "setup") {
    return (
      <>
        <SetupWizard onComplete={() => setState("ready")} />
        <ToastContainer />
      </>
    );
  }

  return (
    <>
      <Shell />
      <ToastContainer />
    </>
  );
}
