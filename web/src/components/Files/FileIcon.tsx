const EXT_ICONS: Record<string, string> = {
  // Images
  png: "\uD83D\uDDBC\uFE0F",
  jpg: "\uD83D\uDDBC\uFE0F",
  jpeg: "\uD83D\uDDBC\uFE0F",
  gif: "\uD83D\uDDBC\uFE0F",
  svg: "\uD83D\uDDBC\uFE0F",
  webp: "\uD83D\uDDBC\uFE0F",
  ico: "\uD83D\uDDBC\uFE0F",
  bmp: "\uD83D\uDDBC\uFE0F",

  // Video
  mp4: "\uD83C\uDFAC",
  mkv: "\uD83C\uDFAC",
  avi: "\uD83C\uDFAC",
  mov: "\uD83C\uDFAC",
  webm: "\uD83C\uDFAC",

  // Audio
  mp3: "\uD83C\uDFB5",
  wav: "\uD83C\uDFB5",
  flac: "\uD83C\uDFB5",
  ogg: "\uD83C\uDFB5",
  aac: "\uD83C\uDFB5",

  // Code
  ts: "\uD83D\uDCBB",
  tsx: "\uD83D\uDCBB",
  js: "\uD83D\uDCBB",
  jsx: "\uD83D\uDCBB",
  py: "\uD83D\uDCBB",
  go: "\uD83D\uDCBB",
  rs: "\uD83D\uDCBB",
  rb: "\uD83D\uDCBB",
  java: "\uD83D\uDCBB",
  c: "\uD83D\uDCBB",
  cpp: "\uD83D\uDCBB",
  h: "\uD83D\uDCBB",
  cs: "\uD83D\uDCBB",
  php: "\uD83D\uDCBB",
  swift: "\uD83D\uDCBB",
  kt: "\uD83D\uDCBB",
  scala: "\uD83D\uDCBB",
  sh: "\uD83D\uDCBB",
  bash: "\uD83D\uDCBB",
  zsh: "\uD83D\uDCBB",
  fish: "\uD83D\uDCBB",
  css: "\uD83D\uDCBB",
  scss: "\uD83D\uDCBB",
  html: "\uD83D\uDCBB",
  vue: "\uD83D\uDCBB",
  svelte: "\uD83D\uDCBB",
  sql: "\uD83D\uDCBB",
  r: "\uD83D\uDCBB",
  lua: "\uD83D\uDCBB",
  zig: "\uD83D\uDCBB",
  nim: "\uD83D\uDCBB",
  ex: "\uD83D\uDCBB",
  exs: "\uD83D\uDCBB",
  erl: "\uD83D\uDCBB",
  hs: "\uD83D\uDCBB",
  ml: "\uD83D\uDCBB",

  // Data / Config
  json: "\uD83D\uDCC4",
  yaml: "\uD83D\uDCC4",
  yml: "\uD83D\uDCC4",
  toml: "\uD83D\uDCC4",
  xml: "\uD83D\uDCC4",
  csv: "\uD83D\uDCC4",
  ini: "\uD83D\uDCC4",
  env: "\uD83D\uDCC4",
  conf: "\uD83D\uDCC4",
  cfg: "\uD83D\uDCC4",

  // Documents
  md: "\uD83D\uDCC3",
  txt: "\uD83D\uDCC3",
  pdf: "\uD83D\uDCC3",
  doc: "\uD83D\uDCC3",
  docx: "\uD83D\uDCC3",
  rtf: "\uD83D\uDCC3",

  // Archives
  zip: "\uD83D\uDCE6",
  tar: "\uD83D\uDCE6",
  gz: "\uD83D\uDCE6",
  bz2: "\uD83D\uDCE6",
  xz: "\uD83D\uDCE6",
  "7z": "\uD83D\uDCE6",
  rar: "\uD83D\uDCE6",

  // Executables / binaries
  exe: "\u2699\uFE0F",
  bin: "\u2699\uFE0F",
  dmg: "\u2699\uFE0F",
  deb: "\u2699\uFE0F",
  rpm: "\u2699\uFE0F",
  AppImage: "\u2699\uFE0F",

  // Docker / infra
  Dockerfile: "\uD83D\uDC33",
  dockerignore: "\uD83D\uDC33",

  // Lock files
  lock: "\uD83D\uDD12",

  // Git
  gitignore: "\uD83D\uDCC2",

  // Makefiles
  Makefile: "\uD83D\uDD27",
  mk: "\uD83D\uDD27",
};

/** Name-based icons for well-known files */
const NAME_ICONS: Record<string, string> = {
  Dockerfile: "\uD83D\uDC33",
  Makefile: "\uD83D\uDD27",
  LICENSE: "\uD83D\uDCC4",
  README: "\uD83D\uDCC3",
  "README.md": "\uD83D\uDCC3",
  ".gitignore": "\uD83D\uDCC2",
  ".dockerignore": "\uD83D\uDC33",
  ".env": "\uD83D\uDD12",
  "go.mod": "\uD83D\uDCBB",
  "go.sum": "\uD83D\uDD12",
  "package.json": "\uD83D\uDCE6",
  "tsconfig.json": "\uD83D\uDCBB",
};

interface FileIconProps {
  name: string;
  isDir: boolean;
  className?: string;
}

export function FileIcon({ name, isDir, className }: FileIconProps) {
  if (isDir) {
    return <span className={className}>{"\uD83D\uDCC1"}</span>;
  }

  // Check by full name first
  if (NAME_ICONS[name]) {
    return <span className={className}>{NAME_ICONS[name]}</span>;
  }

  // Then by extension
  const ext = name.includes(".") ? name.split(".").pop()?.toLowerCase() ?? "" : "";
  const icon = EXT_ICONS[ext] ?? "\uD83D\uDCC4";

  return <span className={className}>{icon}</span>;
}
