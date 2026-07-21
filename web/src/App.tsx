import { useState, useEffect } from "react";
import { Shell } from "@/components/Layout/Shell";
import { SetupWizard } from "@/components/Setup/SetupWizard";
import { ToastContainer } from "@/components/Layout/ToastContainer";
import { AxiosMark } from "@/components/brand/AxiosLogo";
import { AuthGate } from "@/components/Auth/AuthGate";
import { ModelDownloadProvider } from "@/contexts/ModelDownloadContext";
import { ModelDownloadWidget } from "@/components/Layout/ModelDownloadWidget";

type SetupState = "loading" | "setup" | "ready";

function AuthenticatedApp() {
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
          <div className="relative">
            <div className="absolute -inset-2 rounded-2xl border border-primary/20 animate-pulse" />
            <AxiosMark className="w-12 h-12 rounded-xl shadow-[0_0_32px_rgba(120,144,248,0.25)]" />
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
      <ModelDownloadProvider>
        <Shell />
        <ModelDownloadWidget />
      </ModelDownloadProvider>
      <ToastContainer />
    </>
  );
}

export default function App() {
  return (
    <AuthGate>
      <AuthenticatedApp />
    </AuthGate>
  );
}
