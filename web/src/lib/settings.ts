export interface AppSettings {
  systemName: string;
  routingMode: "auto" | "cloud" | "local";
  systemPrompt: string;
  chatPanelWidth: number;
  fileExplorerView: "grid" | "list";
}

export const DEFAULT_SETTINGS: AppSettings = {
  systemName: "AxiOS",
  routingMode: "auto",
  systemPrompt: "",
  chatPanelWidth: 400,
  fileExplorerView: "grid",
};

export function loadSettings(): AppSettings {
  try {
    const raw = localStorage.getItem("axios-settings");
    return raw ? { ...DEFAULT_SETTINGS, ...JSON.parse(raw) } : { ...DEFAULT_SETTINGS };
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export function saveSettings(settings: AppSettings): void {
  localStorage.setItem("axios-settings", JSON.stringify(settings));
  window.dispatchEvent(new CustomEvent<AppSettings>("axios-settings-changed", { detail: settings }));
}
