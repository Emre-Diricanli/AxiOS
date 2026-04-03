import { useState, useEffect } from "react";

type ToastType = "success" | "error" | "info" | "warning";

export interface Toast {
  id: string;
  type: ToastType;
  title: string;
  message?: string;
  duration: number;
  createdAt: number;
}

// Global store
let listeners: Array<(toasts: Toast[]) => void> = [];
let toasts: Toast[] = [];

function notify() {
  listeners.forEach((l) => l([...toasts]));
}

export function toast(
  type: ToastType,
  title: string,
  message?: string,
  duration?: number,
) {
  const id = crypto.randomUUID();
  const dur = duration ?? 4000;
  toasts.push({ id, type, title, message, duration: dur, createdAt: Date.now() });
  // Keep max 5
  if (toasts.length > 5) {
    toasts = toasts.slice(-5);
  }
  notify();
  setTimeout(() => {
    dismissToast(id);
  }, dur);
}

export function dismissToast(id: string) {
  toasts = toasts.filter((t) => t.id !== id);
  notify();
}

// Convenience methods
export const toastSuccess = (title: string, message?: string) =>
  toast("success", title, message);
export const toastError = (title: string, message?: string) =>
  toast("error", title, message);
export const toastInfo = (title: string, message?: string) =>
  toast("info", title, message);
export const toastWarning = (title: string, message?: string) =>
  toast("warning", title, message);

// Hook
export function useToasts(): Toast[] {
  const [state, setState] = useState<Toast[]>([]);
  useEffect(() => {
    listeners.push(setState);
    return () => {
      listeners = listeners.filter((l) => l !== setState);
    };
  }, []);
  return state;
}
