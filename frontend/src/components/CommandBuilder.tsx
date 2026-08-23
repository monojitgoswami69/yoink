"use client";

import React, { useState } from "react";
import { useToast } from "./Toast";

export function CommandBuilder() {
  const [command, setCommand] = useState("init");
  const [target, setTarget] = useState("https://github.com/monojitgoswami69/certify");
  const [healTries, setHealTries] = useState(3);
  const [noAgent, setNoAgent] = useState(false);
  const [noBuild, setNoBuild] = useState(false);
  const { showToast } = useToast();

  let generated = `yoink ${command}`;
  if (command === "init") {
    generated += ` ${target.trim() || "https://github.com/org/repo"}`;
    if (noAgent) generated += " --no-agent";
    if (noBuild) generated += " --no-build";
    if (healTries !== 3) generated += ` --heal-tries ${healTries}`;
  } else if (["up", "down", "status", "logs", "explain", "heal", "restart"].includes(command)) {
    const proj = target.trim().replace(/^https?:\/\/github\.com\/[^/]+\/([^/]+).*$/, "$1") || "my-project";
    generated += ` ${proj}`;
    if (command === "heal" && healTries !== 3) generated += ` --heal-tries ${healTries}`;
  }

  const handleCopy = () => {
    navigator.clipboard.writeText(generated);
    showToast(`✓ Copied: ${generated}`);
  };

  return (
    <div className="cmd-builder" id="cmd-builder">
      <h3 style={{ marginBottom: "0.75rem" }}>Interactive Command Generator</h3>
      <p style={{ fontSize: "0.92rem", color: "var(--text-muted)", marginBottom: "1.25rem" }}>
        Select options to build custom Yoink commands ready to paste into your terminal.
      </p>

      <div className="builder-controls">
        <div className="builder-group">
          <label htmlFor="builder-cmd">Command</label>
          <select
            id="builder-cmd"
            className="builder-select"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
          >
            <option value="init">init (Clone & Run)</option>
            <option value="up">up (Start Stack)</option>
            <option value="down">down (Stop Stack)</option>
            <option value="status">status (Health Check)</option>
            <option value="logs">logs (Stream Logs)</option>
            <option value="heal">heal (Re-run Fixer)</option>
            <option value="explain">explain (Audit Stack)</option>
            <option value="restart">restart (Env Reload)</option>
          </select>
        </div>

        <div className="builder-group">
          <label htmlFor="builder-target">Target Repo URL / Project Name</label>
          <input
            type="text"
            id="builder-target"
            className="builder-input"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          />
        </div>

        <div className="builder-group">
          <label htmlFor="builder-flag-heal-tries">Max Heal Tries</label>
          <input
            type="number"
            id="builder-flag-heal-tries"
            className="builder-input"
            value={healTries}
            min={1}
            max={10}
            onChange={(e) => setHealTries(parseInt(e.target.value, 10) || 3)}
          />
        </div>
      </div>

      <div style={{ display: "flex", gap: "1.5rem", marginBottom: "1.25rem", flexWrap: "wrap" }}>
        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.88rem", fontWeight: 700, cursor: "pointer" }}>
          <input
            type="checkbox"
            id="builder-flag-no-agent"
            checked={noAgent}
            onChange={(e) => setNoAgent(e.target.checked)}
          />
          --no-agent (Static mode, no LLM)
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.88rem", fontWeight: 700, cursor: "pointer" }}>
          <input
            type="checkbox"
            id="builder-flag-no-build"
            checked={noBuild}
            onChange={(e) => setNoBuild(e.target.checked)}
          />
          --no-build (Generate config only)
        </label>
      </div>

      <div className="builder-preview">
        <code id="builder-output-code">{generated}</code>
        <button className="copy-btn copy-btn-sm" onClick={handleCopy}>
          Copy Command
        </button>
      </div>
    </div>
  );
}
