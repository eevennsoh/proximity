import React, { useState, useEffect } from 'react';
import { X, Check } from 'lucide-react';

// Simple markdown-to-react renderer for changelog content
const renderMarkdown = (content) => {
  const lines = content.split('\n');
  const elements = [];
  let inList = false;
  let listItems = [];

  const flushList = () => {
    if (listItems.length > 0) {
      elements.push(
        <ul key={`list-${elements.length}`} className="space-y-2.5">
          {listItems}
        </ul>
      );
      listItems = [];
      inList = false;
    }
  };

  lines.forEach((line, idx) => {
    const trimmedLine = line.trim();
    
    // Skip empty lines
    if (!trimmedLine) {
      flushList();
      return;
    }

    // H3 - Subsection headers like "Features", "Bug Fixes"
    if (trimmedLine.startsWith('### ')) {
      flushList();
      const text = trimmedLine.replace('### ', '');
      // Only add separator if there are already elements (not the first item)
      if (elements.length > 0) {
        elements.push(
          <div key={`sep-${idx}`} className="border-t border-dashed border-white/10 my-4" />
        );
      }
      elements.push(
        <h3 key={idx} className="text-xs font-semibold text-white/50 mb-3 uppercase tracking-wider">
          {text}
        </h3>
      );
      return;
    }

    // List items
    if (trimmedLine.startsWith('- ')) {
      inList = true;
      const text = trimmedLine.replace('- ', '');
      // Handle bold text
      const formattedText = text.split(/(\*\*.*?\*\*)/).map((part, i) => {
        if (part.startsWith('**') && part.endsWith('**')) {
          return <strong key={i} className="text-white/90 font-medium">{part.slice(2, -2)}</strong>;
        }
        return part;
      });
      listItems.push(
        <li key={idx} className="flex items-start gap-3 text-sm text-white/60">
          <span className="flex-shrink-0 mt-0.5 h-4 w-4 rounded-full bg-emerald-500/20 grid place-items-center">
            <Check className="h-2.5 w-2.5 text-emerald-400" strokeWidth={3} />
          </span>
          <span className="leading-relaxed">{formattedText}</span>
        </li>
      );
      return;
    }

    // Regular paragraph
    flushList();
    elements.push(
      <p key={idx} className="text-sm text-white/60 mb-3 leading-relaxed">
        {trimmedLine}
      </p>
    );
  });

  flushList();
  return elements;
};

export default function ChangelogModal({ version, changelog, onClose }) {
  const [isClosing, setIsClosing] = useState(false);

  if (!changelog) return null;

  const handleClose = () => {
    setIsClosing(true);
  };

  const handleAnimationEnd = () => {
    if (isClosing) {
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Backdrop that blocks clicks outside modal but preserves window dragging at top */}
      <div
        className="absolute top-0 left-0 right-0 h-20"
        style={{ "--wails-draggable": "drag" }}
      />
      {/* Rest of backdrop blocks clicks without closing */}
      <div className="absolute inset-x-0 top-20 bottom-0" />
      
      {/* Modal */}
      <div
        className={`relative w-full max-w-md pointer-events-auto transition-all duration-150 ease-out ${
          isClosing
            ? 'opacity-0 scale-95'
            : 'animate-in fade-in zoom-in-95 duration-150'
        }`}
        onTransitionEnd={handleAnimationEnd}
      >
        {/* Main card */}
        <div className="relative bg-[#1a1a1c]/70 backdrop-blur-2xl rounded-2xl overflow-hidden border border-white/[0.08] shadow-lg shadow-black/30">
          
          {/* Header */}
          <div className="relative px-6 pt-6 pb-5">
            {/* Close button */}
            <button
              onClick={handleClose}
              className="absolute top-4 right-4 p-1.5 rounded-lg hover:bg-white/[0.08] transition-colors group"
              aria-label="Close changelog"
            >
              <X className="w-4 h-4 text-white/40 group-hover:text-white/70" />
            </button>
            
            {/* Title and version */}
            <div className="flex items-center gap-2.5">
              <h1 className="text-lg font-semibold text-white">What's New</h1>
              <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-400">
                v{version || '0.0.0'}
              </span>
            </div>
          </div>

          {/* Dotted separator */}
          <div className="mx-6 border-t border-dashed border-white/[0.08]" />

          {/* Content */}
          <div className="px-6 py-5 pb-6 max-h-[45vh] overflow-y-auto no-scrollbar">
            {renderMarkdown(changelog)}
          </div>
        </div>
      </div>
    </div>
  );
}
