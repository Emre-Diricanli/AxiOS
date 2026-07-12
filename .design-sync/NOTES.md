# design-sync notes — AxiOS web UI

- App repo, not a packaged DS: synth-entry from `web/src/components` (srcDir), self-symlink required: `ln -sfn ../../web web/node_modules/axios-web`. **Any `npm install` in web/ prunes the symlink** — recreate before every converter run.
- `main.tsx` mounts the app at module top level — srcDir MUST stay `src/components` (never `src`) or the bundle executes the app mount on load.
- **Dark canvas**: preview template hardcodes white body; `cssEntry` (`web/src/design-canvas.css`) is a GENERATED flat file = compiled `web/dist/assets/index-<hash>.css` + `html/body{background:#08080c!important}` tail. The css pipeline does NOT inline `@import` chains — keep it flat. Regenerate after every `npm run build` in web/ (hash changes): `cat web/dist/assets/index-*.css + dark-canvas tail > web/src/design-canvas.css`.
- Playwright browser: no chromium cache; use system Chrome via `DS_CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"` for validate/capture.
- Fonts: JetBrains Mono ships via devDependency `@fontsource/jetbrains-mono` (cfg.extraFonts). Fira Code is an accepted fallback entry in the same font stack (OFL substitute not shipped).
- `.d.ts` extraction is opaque in synth mode — real contracts are hand-written in cfg.dtsPropsFor (keep in sync with component props when they change).
- Fetch-on-mount components (TasksDrawer, ModelPicker) render their honest empty states in previews ("unavailable"/"No model") — expected, grade on styling.

## Re-sync risks
- `cssEntry` flat file goes stale whenever web/ is rebuilt (hash + new utilities) — regenerate it FIRST on any re-sync.
- `dtsPropsFor` contracts drift silently when component props change (no build error) — diff against src interfaces on re-sync.
- Toolchain: node 24.5, npm; converter deps in .ds-sync (esbuild, ts-morph, @types/react, playwright w/ system Chrome).

## Learnings from preview waves (folded)
- package-capture flags cells whose textContent starts with ⚠ (error-boundary text-prefix sentinel) — ApprovalCard preview carries a hidden marker span workaround.
- Preview classNames are NOT Tailwind-compiled — only classes already in the app bundle exist; use inline styles for preview-only layout.
- UPSTREAM APP BUG found: separator.tsx/tabs.tsx use data-horizontal:/data-vertical: variants that never match (@base-ui emits data-orientation="horizontal"). Separator invisible + Tabs layout broken in the real app; previews carry inline-style workarounds. App fix: add `@custom-variant data-horizontal (&[data-orientation=horizontal])` (+vertical). Remove preview workarounds when fixed.
