"use client";

import React, { useState } from "react";
import { useToast } from "./Toast";

interface CommandItem {
  name: string;
  category: "projects" | "runtime" | "config" | "intelligence" | "diagnostics";
  args: string;
  desc: string;
  flags: string;
}

const COMMANDS_DATA: CommandItem[] = [
  {
    name: "yoink init",
    category: "projects",
    args: "<github-url|path>",
    desc: "Clone repo, detect services, infer infra, generate Dockerfiles, build, heal, and verify.",
    flags: "--name, --no-agent, --force, --output, --no-build, --heal-tries, --max-services"
  },
  {
    name: "yoink setup",
    category: "config",
    args: "",
    desc: "Interactive configuration wizard for LLM provider (OpenAI, Anthropic, Gemini, Groq, Ollama) and GitHub PAT.",
    flags: "--non-interactive"
  },
  {
    name: "yoink list",
    category: "projects",
    args: "",
    desc: "List all initialized projects with container status, port maps, and health.",
    flags: "--running, --stopped"
  },
  {
    name: "yoink up",
    category: "runtime",
    args: "[project]",
    desc: "Start a project stack and wait for container healthchecks to pass.",
    flags: "--detach, --build"
  },
  {
    name: "yoink down",
    category: "runtime",
    args: "[project]",
    desc: "Stop running project containers while preserving named database volumes.",
    flags: "--volumes"
  },
  {
    name: "yoink restart",
    category: "runtime",
    args: "[project]",
    desc: "Restart project containers with environment variable re-rendering.",
    flags: ""
  },
  {
    name: "yoink status",
    category: "runtime",
    args: "[project]",
    desc: "Display live health, running services, endpoints, and container IDs.",
    flags: "--json"
  },
  {
    name: "yoink logs",
    category: "runtime",
    args: "<project> [service]",
    desc: "Stream and follow multi-container logs with unified timestamps.",
    flags: "--follow/-f, --tail N"
  },
  {
    name: "yoink stats",
    category: "runtime",
    args: "<project>",
    desc: "Live CPU, memory usage, and I/O metrics across all services.",
    flags: ""
  },
  {
    name: "yoink open",
    category: "runtime",
    args: "[project]",
    desc: "Open the primary web application endpoint directly in your default browser.",
    flags: ""
  },
  {
    name: "yoink env",
    category: "config",
    args: "<project>",
    desc: "View, set, and override environment variables without re-detecting.",
    flags: "set <key>=<val>, get <key>, list"
  },
  {
    name: "yoink explain",
    category: "intelligence",
    args: "[project]",
    desc: "Print comprehensive audit of detected frameworks, inferred infra, and repair decisions.",
    flags: ""
  },
  {
    name: "yoink heal",
    category: "intelligence",
    args: "[project]",
    desc: "Re-run the autonomous build and heal loop on an existing project.",
    flags: "--heal-tries N"
  },
  {
    name: "yoink update",
    category: "projects",
    args: "[project]",
    desc: "Pull upstream git changes, regenerate configs, rebuild containers, and restart.",
    flags: ""
  },
  {
    name: "yoink incinerate",
    category: "projects",
    args: "<project>",
    desc: "Completely remove project containers, volumes, generated outputs, and state locks.",
    flags: "--force"
  },
  {
    name: "yoink doctor",
    category: "diagnostics",
    args: "",
    desc: "Diagnose local environment (Docker daemon, Compose v2, Git, Go, LLM connectivity).",
    flags: ""
  },
  {
    name: "yoink help",
    category: "diagnostics",
    args: "[command]",
    desc: "Display detailed help and flag usage for any Yoink command.",
    flags: ""
  }
];

export function CommandExplorer() {
  const [activeCategory, setActiveCategory] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");
  const { showToast } = useToast();

  const filteredCommands = COMMANDS_DATA.filter((cmd) => {
    const matchesCategory = activeCategory === "all" || cmd.category === activeCategory;
    const query = searchQuery.trim().toLowerCase();
    const matchesQuery =
      query === "" ||
      cmd.name.toLowerCase().includes(query) ||
      cmd.desc.toLowerCase().includes(query) ||
      cmd.flags.toLowerCase().includes(query);

    return matchesCategory && matchesQuery;
  });

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    showToast(`✓ Copied: ${text}`);
  };

  return (
    <div className="card" style={{ padding: 0, overflow: "hidden" }}>
      <div
        style={{
          padding: "1.5rem",
          backgroundColor: "var(--bg-alt)",
          borderBottom: "var(--border-md)",
          display: "flex",
          flexWrap: "wrap",
          gap: "1rem",
          alignItems: "center",
          justifyContent: "space-between"
        }}
      >
        <div style={{ flex: 1, minWidth: "260px" }}>
          <input
            type="text"
            className="cmd-search-input repo-input"
            placeholder="Search commands (e.g. init, heal, env, stats)..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{ margin: 0, width: "100%" }}
          />
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: "0.4rem" }}>
          {(["all", "projects", "runtime", "config", "intelligence", "diagnostics"] as const).map((cat) => (
            <button
              key={cat}
              className={`preset-chip cmd-filter-pill ${activeCategory === cat ? "active" : ""}`}
              onClick={() => setActiveCategory(cat)}
            >
              {cat.charAt(0).toUpperCase() + cat.slice(1)}
            </button>
          ))}
        </div>
      </div>

      <div className="cmd-list-container">
        {filteredCommands.length === 0 ? (
          <div style={{ padding: "2.5rem", textAlign: "center", color: "var(--text-muted)" }}>
            <p style={{ fontWeight: 800, fontSize: "1.2rem", color: "var(--text-main)" }}>
              No commands matched &quot;{searchQuery}&quot;
            </p>
            <p>
              Try searching for <code>init</code>, <code>heal</code>, <code>setup</code>, or <code>env</code>.
            </p>
          </div>
        ) : (
          filteredCommands.map((cmd) => (
            <div
              key={cmd.name}
              className="cmd-row"
              style={{
                padding: "1.25rem 1.5rem",
                borderBottom: "var(--border-sm)",
                display: "flex",
                flexDirection: "column",
                gap: "0.5rem"
              }}
            >
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "0.5rem" }}>
                <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
                  <span className="cmd-code" style={{ fontSize: "1rem", backgroundColor: "var(--neo-yellow)" }}>
                    {cmd.name} {cmd.args}
                  </span>
                  <span className="badge badge-sm" style={{ fontSize: "0.7rem" }}>
                    {cmd.category}
                  </span>
                </div>
                <button
                  className="copy-btn copy-btn-sm"
                  onClick={() => handleCopy(`${cmd.name} ${cmd.args}`.trim())}
                >
                  Copy
                </button>
              </div>
              <p style={{ margin: 0, fontSize: "0.95rem", color: "var(--text-muted)" }}>{cmd.desc}</p>
              {cmd.flags && (
                <div style={{ fontFamily: "var(--font-mono)", fontSize: "0.8rem", color: "var(--text-subtle)" }}>
                  <span style={{ fontWeight: 700, color: "#000" }}>Flags:</span> {cmd.flags}
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
