type IconSize = "sm" | "md" | "lg";

const SIZE_MAP: Record<IconSize, number> = {
  sm: 16,
  md: 32,
  lg: 48,
};

type FileCategory =
  | "code"
  | "image"
  | "video"
  | "audio"
  | "document"
  | "config"
  | "archive"
  | "executable"
  | "default";

const EXT_CATEGORY: Record<string, FileCategory> = {
  // Code
  ts: "code", tsx: "code", js: "code", jsx: "code",
  py: "code", go: "code", rs: "code", rb: "code",
  java: "code", c: "code", cpp: "code", h: "code",
  cs: "code", php: "code", swift: "code", kt: "code",
  scala: "code", sh: "code", bash: "code", zsh: "code",
  fish: "code", css: "code", scss: "code", html: "code",
  vue: "code", svelte: "code", sql: "code", r: "code",
  lua: "code", zig: "code", nim: "code", ex: "code",
  exs: "code", erl: "code", hs: "code", ml: "code",

  // Images
  png: "image", jpg: "image", jpeg: "image", gif: "image",
  svg: "image", webp: "image", ico: "image", bmp: "image",

  // Video
  mp4: "video", mkv: "video", avi: "video", mov: "video", webm: "video",

  // Audio
  mp3: "audio", wav: "audio", flac: "audio", ogg: "audio", aac: "audio",

  // Documents
  md: "document", txt: "document", pdf: "document",
  doc: "document", docx: "document", rtf: "document",

  // Config
  json: "config", yaml: "config", yml: "config", toml: "config",
  xml: "config", csv: "config", ini: "config", env: "config",
  conf: "config", cfg: "config",

  // Archives
  zip: "archive", tar: "archive", gz: "archive", bz2: "archive",
  xz: "archive", "7z": "archive", rar: "archive",

  // Executables
  exe: "executable", bin: "executable", dmg: "executable",
  deb: "executable", rpm: "executable",
};

const CATEGORY_COLORS: Record<FileCategory, string> = {
  code: "#22c55e",
  image: "#a855f7",
  video: "#ef4444",
  audio: "#f472b6",
  document: "#3b82f6",
  config: "#f59e0b",
  archive: "#eab308",
  executable: "#6b7280",
  default: "#6b7280",
};

const SPECIAL_DIRS = new Set(["Desktop", "Documents", "Downloads", "Pictures", "Music", "Videos"]);

function getFileCategory(name: string): FileCategory {
  const ext = name.includes(".") ? name.split(".").pop()?.toLowerCase() ?? "" : "";
  return EXT_CATEGORY[ext] ?? "default";
}

function getExtLabel(name: string): string | null {
  if (!name.includes(".")) return null;
  const ext = name.split(".").pop()?.toUpperCase() ?? "";
  if (ext.length > 4) return null;
  return ext;
}

interface FileIconProps {
  name: string;
  isDir: boolean;
  size?: IconSize;
  className?: string;
}

export function FileIcon({ name, isDir, size = "md", className = "" }: FileIconProps) {
  const px = SIZE_MAP[size];

  if (isDir) {
    const isSpecial = SPECIAL_DIRS.has(name);
    const gradFrom = isSpecial ? "#7c3aed" : "#4f6ef7";
    const gradTo = isSpecial ? "#a855f7" : "#6987fa";

    return (
      <div
        className={`relative flex-shrink-0 ${className}`}
        style={{ width: px, height: px }}
      >
        <svg viewBox="0 0 48 48" width={px} height={px}>
          {/* Folder tab */}
          <path
            d="M4 14 L4 10 Q4 7 7 7 L18 7 Q20 7 21 9 L23 13 L41 13 Q44 13 44 16 L44 38 Q44 41 41 41 L7 41 Q4 41 4 38 Z"
            fill={`url(#folderGrad-${isSpecial ? "special" : "normal"})`}
            rx="4"
          />
          {/* Folder body */}
          <path
            d="M4 18 L44 18 L44 38 Q44 41 41 41 L7 41 Q4 41 4 38 Z"
            fill={`url(#folderBody-${isSpecial ? "special" : "normal"})`}
            opacity="0.9"
          />
          {/* Shine */}
          <path
            d="M4 18 L44 18 L44 21 L4 21 Z"
            fill="rgba(255,255,255,0.12)"
          />
          <defs>
            <linearGradient id={`folderGrad-${isSpecial ? "special" : "normal"}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={gradTo} />
              <stop offset="100%" stopColor={gradFrom} />
            </linearGradient>
            <linearGradient id={`folderBody-${isSpecial ? "special" : "normal"}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={gradTo} />
              <stop offset="100%" stopColor={gradFrom} stopOpacity="0.85" />
            </linearGradient>
          </defs>
        </svg>
      </div>
    );
  }

  // File icon
  const category = getFileCategory(name);
  const color = CATEGORY_COLORS[category];
  const extLabel = getExtLabel(name);
  const fontSize = size === "sm" ? 4 : size === "md" ? 7 : 9;

  return (
    <div
      className={`relative flex-shrink-0 ${className}`}
      style={{ width: px, height: px }}
    >
      <svg viewBox="0 0 48 56" width={px} height={px}>
        {/* Document body */}
        <path
          d="M4 4 Q4 1 7 1 L32 1 L44 14 L44 52 Q44 55 41 55 L7 55 Q4 55 4 52 Z"
          fill="rgba(200, 200, 220, 0.12)"
          stroke="rgba(255,255,255,0.08)"
          strokeWidth="1"
        />
        {/* Corner fold */}
        <path
          d="M32 1 L32 11 Q32 14 35 14 L44 14 Z"
          fill={color}
          opacity="0.7"
        />
        {/* Color bar at bottom */}
        <rect x="4" y="48" width="40" height="7" rx="0 0 3 3" fill={color} opacity="0.5" />
        {/* Extension label */}
        {extLabel && (
          <text
            x="24"
            y="38"
            textAnchor="middle"
            fontSize={fontSize}
            fontWeight="700"
            fontFamily="system-ui, sans-serif"
            fill={color}
            opacity="0.9"
          >
            {extLabel}
          </text>
        )}
      </svg>
    </div>
  );
}

export { getFileCategory, type FileCategory, type IconSize };
