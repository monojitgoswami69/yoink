"use client";

import React, { useState } from "react";
import Link from "next/link";
import { useToast } from "./Toast";

export function CurlInstallBar() {
  const [copied, setCopied] = useState(false);
  const { showToast } = useToast();
  const installCmd = "curl -sSfL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | sh";

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(installCmd);
      setCopied(true);
      showToast("✓ Copied install command!");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = installCmd;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopied(true);
      showToast("✓ Copied install command!");
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className="install-bar-wrapper">
      <div className="install-bar" role="region" aria-label="Quick install command">
        <div className="install-command">
          <span className="install-prompt">$</span>
          <code>{installCmd}</code>
        </div>
        <button
          className={`btn btn-primary btn-sm copy-btn ${copied ? "copied" : ""}`}
          onClick={handleCopy}
          aria-label="Copy install command"
        >
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
            <rect x="9" y="9" width="13" height="13" rx="2" />
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
          </svg>
          <span className="copy-label">{copied ? "Copied!" : "Copy"}</span>
        </button>
      </div>
      <div className="install-meta" style={{ justifyContent: "center" }}>
        <span>⚡ Auto-detects Linux, macOS (Apple Silicon & Intel), Windows</span>
        <span>•</span>
        <Link href="/docs#installation" style={{ textDecoration: "underline", color: "#000" }}>
          View all install methods →
        </Link>
      </div>
    </div>
  );
}
