import React from "react";
import { CodeBlock } from "./CodeBlock";

export function FeatureBento() {
  return (
    <div className="bento-grid">
      {/* CARD 1: HEALING LOOP */}
      <div className="card bento-col-8">
        <div className="card-header card-header-yellow">
          <span>01. BOUNDED AI HEALING LOOP</span>
          <span className="badge badge-sm badge-green">DEFAULT: 3 TRIES</span>
        </div>
        <h3>Autonomous Build Repair with Independent Verification</h3>
        <p>
          When a Docker build fails, Yoink extracts the high-priority root cause (TypeScript TS errors, npm mismatches, Python ImportError) rather than downstream noise. The bounded agent patches generated Dockerfiles, rebuilds, and independently verifies HTTP 200 reachability.
        </p>
        <div style={{ marginTop: "auto" }}>
          <CodeBlock
            code={`[1/3] Root cause: Vite build failed (missing rollup native binary)\n[1/3] Patch applied: Added libc6-compat to Dockerfile.service-1\n[1/3] Rebuilding... Succeeded! Verification: http://localhost:80 (HTTP 200)`}
            headerTitle="$ yoink heal <project> --heal-tries 5"
          />
        </div>
      </div>

      {/* CARD 2: INFRASTRUCTURE */}
      <div className="card bento-col-4">
        <div className="card-header card-header-green">
          <span>02. INFRASTRUCTURE</span>
        </div>
        <h3>Infers 6 Backing Services</h3>
        <p>
          Scans env vars and SDKs. Sees <code>DATABASE_URL</code>? Provisions Postgres with healthchecks. Detects Upstash or Neon? Connects directly without local overhead.
        </p>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "0.5rem", marginTop: "auto" }}>
          <div className="mini-chip">🐘 Postgres 16</div>
          <div className="mini-chip">⚡ Redis 7</div>
          <div className="mini-chip">🍃 MongoDB 7</div>
          <div className="mini-chip">🐬 MySQL 8</div>
          <div className="mini-chip">🐇 RabbitMQ 3</div>
          <div className="mini-chip">🔍 Elasticsearch</div>
        </div>
      </div>

      {/* CARD 3: DETECTOR ENGINE */}
      <div className="card bento-col-4">
        <div className="card-header card-header-pink">
          <span>03. DETECTOR ENGINE</span>
        </div>
        <h3>14 Framework Profiles</h3>
        <p>
          Next.js, Vite, FastAPI, Django, NestJS, Astro, SvelteKit, and more receive bespoke, optimized multi-stage Dockerfiles rather than bloated generic images.
        </p>
      </div>

      {/* CARD 4: PERSISTENT STATE */}
      <div className="card bento-col-4">
        <div className="card-header card-header-purple">
          <span>04. PERSISTENT STATE</span>
        </div>
        <h3>Zero-Input Day-2 Re-Runs</h3>
        <p>
          Stores authoritative state in <code>~/.yoink/state/&lt;project&gt;/yoink.lock</code>. Subsequent runs of <code>yoink up</code>, <code>yoink restart</code>, and <code>yoink status</code> require zero re-detection.
        </p>
      </div>

      {/* CARD 5: PROVIDER AGNOSTIC */}
      <div className="card bento-col-4">
        <div className="card-header card-header-cyan">
          <span>05. PROVIDER AGNOSTIC</span>
        </div>
        <h3>5 Providers + Offline Ollama</h3>
        <p>
          Configure OpenAI, Anthropic Claude, Google Gemini, Groq, or 100% local Ollama. Or run in pure static mode with <code>--no-agent</code> (zero API keys needed).
        </p>
      </div>

      {/* CARD 6: SECURITY & SAFEEFS */}
      <div className="card bento-col-12">
        <div className="card-header card-header-orange">
          <span>06. SECURITY &amp; SAFEEFS SANDBOXING</span>
          <span className="badge badge-sm badge-yellow">STRICT PRIVACY</span>
        </div>
        <div className="grid grid-3" style={{ gap: "1.25rem", marginTop: "0.25rem" }}>
          <div style={{ backgroundColor: "var(--bg-main)", padding: "1.25rem", border: "var(--border-sm)", borderRadius: "var(--radius-sm)", boxShadow: "var(--shadow-sm)" }}>
            <h4 style={{ fontSize: "1rem", marginBottom: "0.5rem" }}>🔒 Sandboxed SafeFS Reader</h4>
            <p style={{ fontSize: "0.88rem", margin: 0, color: "var(--text-muted)", lineHeight: 1.5 }}>
              Agent file reads are strictly limited (max 5 files/round, 16 KiB caps) and reject path traversal or symlinks outside the repository.
            </p>
          </div>
          <div style={{ backgroundColor: "var(--bg-main)", padding: "1.25rem", border: "var(--border-sm)", borderRadius: "var(--radius-sm)", boxShadow: "var(--shadow-sm)" }}>
            <h4 style={{ fontSize: "1rem", marginBottom: "0.5rem" }}>🛡️ Immutable Source Code</h4>
            <p style={{ fontSize: "0.88rem", margin: 0, color: "var(--text-muted)", lineHeight: 1.5 }}>
              The agent can only edit generated files in <code>yoink-outputs/</code>. Your repository source files are never altered.
            </p>
          </div>
          <div style={{ backgroundColor: "var(--bg-main)", padding: "1.25rem", border: "var(--border-sm)", borderRadius: "var(--radius-sm)", boxShadow: "var(--shadow-sm)" }}>
            <h4 style={{ fontSize: "1rem", marginBottom: "0.5rem" }}>🔑 Zero Token Leakage</h4>
            <p style={{ fontSize: "0.88rem", margin: 0, color: "var(--text-muted)", lineHeight: 1.5 }}>
              Git tokens injected via header flags in-memory only. PATs never hit disk, stdout, or CLI arguments.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
