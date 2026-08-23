"use client";

import React, { useState, useEffect, useRef } from "react";

const TERMINAL_STEPS = [
  {
    step: 0,
    text: `<span class="t-prompt">$</span> <span class="t-cmd">yoink init https://github.com/monojitgoswami69/certify</span>\n`,
    delay: 400
  },
  {
    step: 1,
    text: `<span class="t-step">[1/8] Clone repository</span>          <span class="t-success">cloned (81 files)</span>\n`,
    delay: 500
  },
  {
    step: 2,
    text: `<span class="t-step">[2/8] Generate repository tree</span>   <span class="t-success">Tree generated (depth: 4)</span>\n`,
    delay: 400
  },
  {
    step: 3,
    text: `<span class="t-step">[3/8] Detect services</span>            <span class="t-success">vite frontend detected</span> (confidence: high)\n`,
    delay: 600
  },
  {
    step: 4,
    text: `<span class="t-step">[4/8] Extract environment vars</span>  <span class="t-dim">0 variable references</span>\n`,
    delay: 350
  },
  {
    step: 5,
    text: `<span class="t-step">[5/8] Infer backing services</span>     <span class="t-dim">none required</span>\n`,
    delay: 350
  },
  {
    step: 6,
    text: `<span class="t-step">[6/8] Generate Docker config</span>     <span class="t-success">Dockerfile.service-1 + compose</span>\n`,
    delay: 550
  },
  {
    step: 7,
    text: `<span class="t-step">[7/8] Write outputs</span>              <span class="t-success">.env.example + .dockerignore</span>\n`,
    delay: 400
  },
  {
    step: 8,
    text: `<span class="t-step">[8/8] Build & heal</span>              <span class="t-agent">Running docker compose build...</span>\n` +
          `<span class="t-warn">  ↳ Build attempt 1: TypeScript TS2307 (missing @types/node)</span>\n` +
          `<span class="t-agent">  ↳ Agent self-heal [1/3]: Proposing patch to Dockerfile (npm install -D @types/node)</span>\n` +
          `<span class="t-success">  ↳ Rebuilding... Build succeeded!</span>\n`,
    delay: 1100
  },
  {
    step: 9,
    text: `\n<span class="t-success">✓ Init complete in 10.3s</span>\n` +
          `  <span class="t-dim">Stack status:</span> <span class="t-success">RUNNING (healthy)</span>\n` +
          `  <span class="t-dim">Local endpoint:</span> <a href="http://localhost:80/" target="_blank" class="t-url">http://localhost:80/</a>  <span class="t-success">(HTTP 200 OK)</span>\n` +
          `\n<span class="t-prompt">$</span> <span class="t-cursor"></span>`,
    delay: 600
  }
];

export function TerminalSimulator() {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  const [terminalHtml, setTerminalHtml] = useState("");
  const bodyRef = useRef<HTMLDivElement>(null);
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  const startReplay = () => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    setTerminalHtml("");
    setCurrentIndex(0);
    setIsPlaying(true);
  };

  const togglePause = () => {
    setIsPlaying((prev) => !prev);
  };

  useEffect(() => {
    if (!isPlaying) {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      return;
    }

    if (currentIndex >= TERMINAL_STEPS.length) return;

    const currentStep = TERMINAL_STEPS[currentIndex];
    
    timeoutRef.current = setTimeout(() => {
      setTerminalHtml((prev) => {
        const stripped = prev.replace('<span class="t-cursor"></span>', '');
        return stripped + currentStep.text;
      });
      setCurrentIndex((prev) => prev + 1);
    }, currentStep.delay);

    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, [currentIndex, isPlaying]);

  useEffect(() => {
    if (bodyRef.current) {
      bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
    }
  }, [terminalHtml]);

  const total = TERMINAL_STEPS.length - 1;
  const progress = Math.min(100, Math.round((currentIndex / total) * 100));
  const stageNum = Math.min(8, currentIndex);

  return (
    <div className="terminal-window" style={{ maxWidth: "960px", margin: "2rem auto 0" }}>
      <div className="terminal-header">
        <div className="terminal-controls">
          <span className="terminal-dot dot-red"></span>
          <span className="terminal-dot dot-yellow"></span>
          <span className="terminal-dot dot-green"></span>
        </div>
        <div className="terminal-title">
          <span>yoink-session: certify (interactive replay)</span>
        </div>
        <div className="terminal-actions">
          <button className="terminal-btn" onClick={togglePause}>
            {isPlaying ? "⏸ Pause" : "▶ Resume"}
          </button>
          <button className="terminal-btn" onClick={startReplay}>
            ↻ Restart
          </button>
        </div>
      </div>

      <div
        className="terminal-body"
        ref={bodyRef}
        dangerouslySetInnerHTML={{ __html: terminalHtml }}
        aria-live="polite"
      />

      <div className="terminal-scrubber">
        <div className="progress-track">
          <div className="progress-bar-fill" style={{ width: `${progress}%` }}></div>
        </div>
        <span className="step-indicator">Stage: {stageNum} / 8</span>
      </div>
    </div>
  );
}
