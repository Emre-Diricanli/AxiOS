import { useEffect, useCallback, useState } from "react";

interface ImageViewerProps {
  images: { name: string; path: string }[];
  currentIndex: number;
  onClose: () => void;
  onNavigate: (index: number) => void;
}

export function ImageViewer({ images, currentIndex, onClose, onNavigate }: ImageViewerProps) {
  const [zoom, setZoom] = useState(false);

  const current = images[currentIndex];
  const hasPrev = currentIndex > 0;
  const hasNext = currentIndex < images.length - 1;

  const goPrev = useCallback(() => {
    if (hasPrev) onNavigate(currentIndex - 1);
  }, [hasPrev, currentIndex, onNavigate]);

  const goNext = useCallback(() => {
    if (hasNext) onNavigate(currentIndex + 1);
  }, [hasNext, currentIndex, onNavigate]);

  // Keyboard navigation
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowLeft") goPrev();
      if (e.key === "ArrowRight") goNext();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose, goPrev, goNext]);

  // Reset zoom on image change
  useEffect(() => {
    setZoom(false);
  }, [currentIndex]);

  if (!current) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/85 backdrop-blur-md" />

      {/* Header */}
      <div className="absolute top-0 left-0 right-0 flex items-center justify-between px-6 py-4 z-10">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-white">{current.name}</span>
          <span className="text-xs text-white/40 font-mono">
            {currentIndex + 1} / {images.length}
          </span>
        </div>
        <button
          onClick={(e) => { e.stopPropagation(); onClose(); }}
          className="w-8 h-8 rounded-lg bg-white/10 hover:bg-white/20 flex items-center justify-center transition-colors"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="white" strokeWidth="1.5">
            <path d="M4 4l8 8M12 4l-8 8" />
          </svg>
        </button>
      </div>

      {/* Image */}
      <img
        src={`/api/fs/raw?path=${encodeURIComponent(current.path)}`}
        alt={current.name}
        onClick={(e) => { e.stopPropagation(); setZoom(!zoom); }}
        className={`relative z-10 max-h-[85vh] rounded-lg shadow-2xl transition-all duration-200 pointer-events-auto ${
          zoom
            ? "max-w-none cursor-zoom-out scale-150"
            : "max-w-[85vw] cursor-zoom-in"
        }`}
        style={{ objectFit: "contain" }}
      />

      {/* Navigation buttons — on top of everything */}
      {hasPrev && (
        <button
          onClick={(e) => { e.stopPropagation(); goPrev(); }}
          className="absolute left-4 top-1/2 -translate-y-1/2 z-30 w-11 h-11 rounded-full bg-black/60 hover:bg-black/80 flex items-center justify-center transition-colors backdrop-blur-sm border border-white/10"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
      )}
      {hasNext && (
        <button
          onClick={(e) => { e.stopPropagation(); goNext(); }}
          className="absolute right-4 top-1/2 -translate-y-1/2 z-30 w-11 h-11 rounded-full bg-black/60 hover:bg-black/80 flex items-center justify-center transition-colors backdrop-blur-sm border border-white/10"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M9 18l6-6-6-6" />
          </svg>
        </button>
      )}

      {/* Thumbnail strip */}
      {images.length > 1 && (
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 flex items-center gap-1.5 px-3 py-2 rounded-xl bg-black/50 backdrop-blur-md max-w-[80vw] overflow-x-auto scrollbar-none">
          {images.map((img, i) => (
            <button
              key={img.path}
              onClick={(e) => { e.stopPropagation(); onNavigate(i); }}
              className={`shrink-0 w-10 h-10 rounded-md overflow-hidden border-2 transition-all ${
                i === currentIndex
                  ? "border-primary shadow-[0_0_8px_rgba(99,102,241,0.4)]"
                  : "border-transparent opacity-50 hover:opacity-80"
              }`}
            >
              <img
                src={`/api/fs/raw?path=${encodeURIComponent(img.path)}`}
                alt={img.name}
                className="w-full h-full object-cover"
              />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
