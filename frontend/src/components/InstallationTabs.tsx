"use client";

import React, { useState } from "react";
import Link from "next/link";
import { CodeBlock } from "./CodeBlock";

export function InstallationTabs() {
  const [activeTab, setActiveTab] = useState<"curl" | "github" | "source">("curl");

  return (
    <div className="tabs-container">
      <div className="tabs-nav">
        <button
          className={`tab-btn ${activeTab === "curl" ? "active active-yellow" : ""}`}
          onClick={() => setActiveTab("curl")}
        >
          One-Line Script (Recommended)
        </button>
        <button
          className={`tab-btn ${activeTab === "github" ? "active active-green" : ""}`}
          onClick={() => setActiveTab("github")}
        >
          GitHub Releases
        </button>
        <button
          className={`tab-btn ${activeTab === "source" ? "active active-purple" : ""}`}
          onClick={() => setActiveTab("source")}
        >
          Build from Source
        </button>
      </div>

      {activeTab === "curl" && (
        <div className="tab-pane active" id="tab-curl">
          <h3 style={{ marginBottom: "0.75rem" }}>Universal One-Line Installer</h3>
          <p>
            The recommended way to install Yoink on macOS and Linux. Detects your OS and architecture automatically:
          </p>

          <CodeBlock
            code="curl -sSfL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | bash"
            headerTitle="Shell Installer"
          />

          <div className="callout callout-tip">
            <div className="callout-header">💡 Shell Configuration</div>
            <p style={{ margin: 0, fontSize: "0.95rem" }}>
              If <code>~/.local/bin</code> is not in your PATH, add <code>export PATH=&quot;$HOME/.local/bin:$PATH&quot;</code> to your <code>~/.zshrc</code> or <code>~/.bashrc</code>.
            </p>
          </div>
        </div>
      )}

      {activeTab === "github" && (
        <div className="tab-pane active" id="tab-github">
          <h3 style={{ marginBottom: "0.75rem" }}>Pre-Built Release Binaries</h3>
          <p>Download official archives directly from GitHub Releases for your specific OS and architecture:</p>
          <ul style={{ margin: "1rem 0 1.5rem 1.5rem", fontFamily: "var(--font-mono)", fontSize: "0.95rem" }}>
            <li>
              <a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer" style={{ textDecoration: "underline", fontWeight: 700 }}>
                yoink_Darwin_arm64.tar.gz
              </a> (Apple Silicon M1/M2/M3/M4)
            </li>
            <li>
              <a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer" style={{ textDecoration: "underline", fontWeight: 700 }}>
                yoink_Darwin_x86_64.tar.gz
              </a> (macOS Intel)
            </li>
            <li>
              <a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer" style={{ textDecoration: "underline", fontWeight: 700 }}>
                yoink_Linux_x86_64.tar.gz
              </a> (Linux 64-bit)
            </li>
            <li>
              <a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer" style={{ textDecoration: "underline", fontWeight: 700 }}>
                yoink_Linux_arm64.tar.gz
              </a> (Linux ARM64)
            </li>
            <li>
              <a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer" style={{ textDecoration: "underline", fontWeight: 700 }}>
                yoink_Windows_x86_64.zip
              </a> (Windows 64-bit)
            </li>
          </ul>
        </div>
      )}

      {activeTab === "source" && (
        <div className="tab-pane active" id="tab-source">
          <h3 style={{ marginBottom: "0.75rem" }}>Compile from Source with Go</h3>
          <p>Requires Go 1.25+ installed on your system:</p>
          <CodeBlock
            code={`git clone https://github.com/monojitgoswami69/yoink.git\ncd yoink\ngo build -ldflags "-s -w" -o yoink .\nmv yoink /usr/local/bin/   # or ~/.local/bin`}
            headerTitle="Build from source"
          />
          <p style={{ fontSize: "0.95rem" }}>
            Or check out the comprehensive{" "}
            <Link href="/docs#getting-started" style={{ fontWeight: 700, textDecoration: "underline" }}>
              Build &amp; Compilation Guide in Documentation →
            </Link>
          </p>
        </div>
      )}
    </div>
  );
}
