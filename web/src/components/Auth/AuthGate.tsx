import {
  createContext,
  type FormEvent,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  AlertCircle,
  ArrowRight,
  Eye,
  EyeOff,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
  TerminalSquare,
} from "lucide-react";
import { AxiosLogo, AxiosMark } from "@/components/brand/AxiosLogo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface AuthStatus {
  auth_required: boolean;
  authenticated: boolean;
}

interface AuthContextValue {
  authRequired: boolean;
  logout: () => Promise<void>;
}

type GateState = "checking" | "login" | "ready" | "unavailable";

const UNAUTHORIZED_EVENT = "axios-auth-required";
const AuthContext = createContext<AuthContextValue | null>(null);

function requestPath(input: RequestInfo | URL): string {
  const value = input instanceof Request ? input.url : String(input);
  return new URL(value, window.location.href).pathname;
}

function shouldIgnoreUnauthorized(path: string): boolean {
  return path === "/api/auth/login" || path === "/api/auth/status";
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used inside AuthGate");
  }
  return context;
}

export function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<GateState>("checking");
  const [authRequired, setAuthRequired] = useState(true);
  const [statusError, setStatusError] = useState<string | null>(null);

  const checkStatus = useCallback(async () => {
    setState("checking");
    setStatusError(null);

    try {
      const response = await fetch("/api/auth/status", {
        headers: { Accept: "application/json" },
      });
      if (!response.ok) {
        throw new Error(`Status check failed (${response.status})`);
      }

      const status = (await response.json()) as AuthStatus;
      setAuthRequired(status.auth_required);
      setState(status.auth_required && !status.authenticated ? "login" : "ready");
    } catch (error) {
      setStatusError(error instanceof Error ? error.message : "Unable to reach axiosd");
      setState("unavailable");
    }
  }, []);

  useEffect(() => {
    const nativeFetch = window.fetch.bind(window);
    const guardedFetch: typeof window.fetch = async (...args) => {
      const response = await nativeFetch(...args);
      if (response.status === 401 && !shouldIgnoreUnauthorized(requestPath(args[0]))) {
        window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
      }
      return response;
    };

    window.fetch = guardedFetch;
    return () => {
      if (window.fetch === guardedFetch) {
        window.fetch = nativeFetch;
      }
    };
  }, []);

  useEffect(() => {
    const requireLogin = () => {
      setAuthRequired(true);
      setState("login");
    };
    window.addEventListener(UNAUTHORIZED_EVENT, requireLogin);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, requireLogin);
  }, []);

  useEffect(() => {
    void checkStatus();
  }, [checkStatus]);

  const logout = useCallback(async () => {
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } finally {
      if (authRequired) {
        setState("login");
      } else {
        await checkStatus();
      }
    }
  }, [authRequired, checkStatus]);

  if (state === "checking") {
    return <AuthLoading />;
  }

  if (state === "unavailable") {
    return <AuthUnavailable error={statusError} onRetry={checkStatus} />;
  }

  if (state === "login") {
    return <LoginScreen onAuthenticated={() => setState("ready")} />;
  }

  return (
    <AuthContext.Provider value={{ authRequired, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

function AuthLoading() {
  return (
    <main className="min-h-screen grid place-items-center bg-background" aria-busy="true">
      <div className="flex flex-col items-center gap-4">
        <AxiosMark className="size-12 rounded-xl" />
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircle className="size-4 animate-spin text-primary" />
          Verifying secure session
        </div>
      </div>
    </main>
  );
}

function AuthUnavailable({ error, onRetry }: { error: string | null; onRetry: () => void }) {
  return (
    <main className="min-h-screen grid place-items-center bg-background px-5">
      <section className="surface-panel w-full max-w-md rounded-xl p-6 text-center shadow-2xl shadow-black/30">
        <div className="mx-auto mb-4 grid size-11 place-items-center rounded-lg border border-destructive/25 bg-destructive/10 text-destructive">
          <AlertCircle className="size-5" />
        </div>
        <h1 className="text-lg font-semibold text-foreground">Unable to reach AxiOS</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          The interface could not verify the axiosd authentication status. Confirm the daemon is running, then try again.
        </p>
        {error && <p className="mt-3 font-mono text-xs text-destructive/80">{error}</p>}
        <Button className="mt-5 w-full" size="lg" onClick={onRetry}>
          <RefreshCw className="size-4" />
          Retry connection
        </Button>
      </section>
    </main>
  );
}

function LoginScreen({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [token, setToken] = useState("");
  const [showToken, setShowToken] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const cleanToken = token.trim();
    if (!cleanToken) {
      setError("Enter the admin token to continue.");
      inputRef.current?.focus();
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ token: cleanToken }),
      });

      if (response.ok) {
        setToken("");
        onAuthenticated();
        return;
      }

      if (response.status === 401) {
        setError("That admin token is not valid.");
      } else if (response.status === 429) {
        setError("Too many attempts. Wait one minute, then try again.");
      } else if (response.status === 403) {
        setError("This browser origin is not allowed by axiosd.");
      } else {
        const body = (await response.json().catch(() => null)) as { error?: string } | null;
        setError(body?.error || `Sign-in failed (${response.status}).`);
      }
    } catch {
      setError("Could not connect to axiosd. Check that the daemon is running.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="relative min-h-screen overflow-hidden bg-background text-foreground">
      <div className="pointer-events-none absolute inset-0 auth-grid" aria-hidden="true" />
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-primary/55 to-transparent" aria-hidden="true" />

      <header className="relative z-10 flex h-16 items-center justify-between border-b border-border/70 px-6 max-[640px]:px-4">
        <AxiosLogo markClassName="size-8" wordmarkClassName="text-base" />
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="size-1.5 rounded-full bg-emerald-400" />
          Local control plane
        </div>
      </header>

      <div className="relative z-10 mx-auto grid min-h-[calc(100vh-4rem)] w-full max-w-6xl grid-cols-[minmax(0,1fr)_430px] items-center gap-20 px-8 py-12 max-[900px]:max-w-md max-[900px]:grid-cols-1 max-[900px]:px-5">
        <section className="max-w-xl max-[900px]:hidden">
          <div className="mb-7 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-3 py-1.5 text-xs font-medium text-primary">
            <ShieldCheck className="size-3.5" />
            Private infrastructure workspace
          </div>
          <h1 className="text-balance text-4xl font-semibold leading-[1.12] tracking-[-0.035em] text-foreground">
            Your machines. Your models. Your control plane.
          </h1>
          <p className="mt-5 max-w-lg text-[15px] leading-7 text-muted-foreground">
            AxiOS keeps system operations and local inference behind one administrator session. Your token is exchanged for a secure, HTTP-only browser session and is never stored by the interface.
          </p>

          <div className="mt-10 grid grid-cols-2 gap-3">
            <div className="surface-panel rounded-lg p-4">
              <TerminalSquare className="mb-5 size-5 text-primary" />
              <p className="text-sm font-medium">One control surface</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">Hosts, containers, files, models, and coding agents.</p>
            </div>
            <div className="surface-panel rounded-lg p-4">
              <LockKeyhole className="mb-5 size-5 text-primary" />
              <p className="text-sm font-medium">Session protected</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">Same-origin cookies protect API and WebSocket access.</p>
            </div>
          </div>
        </section>

        <section className="surface-panel rounded-xl shadow-2xl shadow-black/40">
          <div className="border-b border-border px-7 py-6 max-[480px]:px-5">
            <div className="mb-5 grid size-10 place-items-center rounded-lg border border-primary/25 bg-primary/10 text-primary">
              <KeyRound className="size-5" />
            </div>
            <h2 className="text-xl font-semibold tracking-tight">Administrator access</h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              Enter the admin token generated by axiosd.
            </p>
          </div>

          <form onSubmit={submit} className="px-7 py-6 max-[480px]:px-5">
            <label htmlFor="admin-token" className="mb-2 block text-xs font-medium text-foreground/85">
              Admin token
            </label>
            <div className="relative">
              <Input
                ref={inputRef}
                id="admin-token"
                type={showToken ? "text" : "password"}
                value={token}
                onChange={(event) => {
                  setToken(event.target.value);
                  if (error) setError(null);
                }}
                placeholder="axsk_..."
                autoComplete="current-password"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                aria-invalid={Boolean(error)}
                aria-describedby={error ? "auth-error" : "token-help"}
                className="h-11 pr-11 font-mono text-sm"
                disabled={submitting}
              />
              <button
                type="button"
                onClick={() => setShowToken((visible) => !visible)}
                className="absolute right-1.5 top-1/2 grid size-8 -translate-y-1/2 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                aria-label={showToken ? "Hide admin token" : "Show admin token"}
              >
                {showToken ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>

            <div className="min-h-12 pt-2.5" aria-live="polite">
              {error ? (
                <p id="auth-error" role="alert" className="flex items-start gap-2 text-xs leading-5 text-destructive">
                  <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
                  {error}
                </p>
              ) : (
                <p id="token-help" className="text-xs leading-5 text-muted-foreground">
                  Find it in the axiosd startup log. If it is lost, reset authentication from the host terminal.
                </p>
              )}
            </div>

            <Button type="submit" size="lg" className="h-10 w-full" disabled={submitting}>
              {submitting ? (
                <>
                  <LoaderCircle className="size-4 animate-spin" />
                  Verifying token
                </>
              ) : (
                <>
                  Continue to AxiOS
                  <ArrowRight className="size-4" />
                </>
              )}
            </Button>
          </form>

          <div className="flex items-center gap-2 border-t border-border px-7 py-4 text-[11px] text-muted-foreground max-[480px]:px-5">
            <ShieldCheck className="size-3.5 text-emerald-400" />
            Token exchange secured by same-origin session controls
          </div>
        </section>
      </div>
    </main>
  );
}
