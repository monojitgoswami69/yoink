"use client";

import React, { useState } from "react";
import { useToast } from "./Toast";

interface CodeBlockProps {
  code: string;
  headerTitle?: string;
  className?: string;
}

export function CodeBlock({ code, headerTitle, className = "" }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);
  const { showToast } = useToast();

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      showToast("✓ Copied to clipboard!");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = code;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopied(true);
      showToast("✓ Copied to clipboard!");
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className={`code-block ${className}`}>
      <div className="code-header">
        <span>{headerTitle || "Terminal"}</span>
        <button
          className={`copy-btn copy-btn-sm ${copied ? "copied" : ""}`}
          onClick={handleCopy}
          aria-label="Copy code to clipboard"
        >
          {copied ? "✓ Copied" : "Copy"}
        </button>
      </div>
      <pre className="code-content">
        <code>{code}</code>
      </pre>
    </div>
  );
}
