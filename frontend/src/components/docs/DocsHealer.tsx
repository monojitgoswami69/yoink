import React from "react";
import { CodeBlock } from "@/components/CodeBlock";

export function DocsHealer() {
  return (
    <div>
      <section id="healer-loop">
        <h2>The 6-Step Autonomous Build &amp; Self-Healing Loop</h2>
        <p>
          The differentiator of Yoink. When <code>docker compose build</code> returns a non-zero exit code, Yoink activates its autonomous repair loop:
        </p>

        <ol>
          <li><strong>Raw Log Capture</strong>: Captures stderr/stdout transcripts directly from Docker BuildKit.</li>
          <li><strong>Root-Cause Prioritization</strong>: Scans and ranks high-priority compiler/package errors over noisy downstream warnings.</li>
          <li><strong>Deterministic Fixers</strong>: Tries instant rule-based fixes first (e.g. adding <code>libc6-compat</code> for Alpine, pinning Python version).</li>
          <li><strong>Bounded AI Investigation</strong>: If deterministic fixes fail, the LLM reads relevant configuration files within strict sandboxed limits.</li>
          <li><strong>Constrained Patching</strong>: Patches are applied exclusively to generated Dockerfiles in <code>yoink-outputs/</code>.</li>
          <li><strong>Rebuild &amp; Independent Verification</strong>: Rebuilds the image and runs live HTTP health checks. The AI never decides whether a build succeeded — runtime verification does.</li>
        </ol>
      </section>

      <section id="root-cause-ranking" style={{ marginTop: "3rem" }}>
        <h2>High-Priority Error Ranking Heuristics</h2>
        <p>Yoink ranks compiler errors using high-specificity pattern matchers:</p>
        <ul>
          <li><strong>TypeScript Errors (<code>TSxxxx</code>)</strong>: Missing types, incompatible tsconfig target versions.</li>
          <li><strong>Node/NPM Errors</strong>: Native addon compilation failures (e.g. <code>rollup</code>, <code>sharp</code>, <code>node-gyp</code>), missing <code>devDependencies</code> during build.</li>
          <li><strong>Python Errors</strong>: Missing C-compiler headers (e.g. <code>gcc</code>, <code>musl-dev</code>, <code>libpq-dev</code>), pip dependency version conflicts.</li>
          <li><strong>Go Errors</strong>: CGO cross-compilation errors, missing system libraries.</li>
        </ul>
      </section>

      <section id="safefs-security" style={{ marginTop: "3rem" }}>
        <h2>SafeFS &amp; Privacy Sandboxing Guarantees</h2>
        <div className="callout callout-important">
          <div className="callout-header">🔒 Security Guarantees</div>
          <ul style={{ margin: 0, paddingLeft: "1.2rem", fontSize: "0.95rem" }}>
            <li><strong>Read Bounds</strong>: Maximum 5 file reads per round, capped at 16 KiB per file.</li>
            <li><strong>Path Traversal Block</strong>: Rejects any relative paths attempting to escape repository root (<code>../</code>) or symlinks pointing outside.</li>
            <li><strong>Write Restriction</strong>: The agent is strictly forbidden from modifying repository source code. Only generated assets in <code>yoink-outputs/</code> may be patched.</li>
            <li><strong>Zero Token Leakage</strong>: GitHub PATs are passed via in-memory transport flags only and never written to disk or stdout.</li>
          </ul>
        </div>
      </section>

      <section id="persistent-state" style={{ marginTop: "3rem" }}>
        <h2>Persistent State Directory (<code>~/.yoink/state/</code>)</h2>
        <p>Every project&apos;s metadata and lockfile are stored in <code>~/.yoink/state/&lt;project&gt;/</code>:</p>
        <CodeBlock
          code={`~/.yoink/state/myproject/\n├── yoink.lock           # Authoritative state: service map, detection hash, ports\n├── env-overrides.json   # User modifications applied during yoink up\n└── settings.json        # Project-specific preferences`}
          headerTitle="~/.yoink/state/<project>/ Structure"
        />
      </section>
    </div>
  );
}
