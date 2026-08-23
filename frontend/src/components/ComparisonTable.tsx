import React from "react";

export function ComparisonTable() {
  return (
    <div className="compare-box">
      <div className="compare-head">
        <div>Capability</div>
        <div>docker init</div>
        <div>Buildpacks / Nix</div>
        <div>Yoink CLI</div>
      </div>
      <div className="compare-row">
        <div>
          <strong>Multi-Service Monorepo Detection</strong>
          <br />
          <small>Detects frontend + backend + worker simultaneously</small>
        </div>
        <div>✖ Single only</div>
        <div>⚠️ Partial</div>
        <div>
          <span className="highlight-cell">✔ Full monorepos</span>
        </div>
      </div>
      <div className="compare-row">
        <div>
          <strong>Backing Infrastructure Inference</strong>
          <br />
          <small>Auto-provisions Postgres, Redis, Mongo from code refs</small>
        </div>
        <div>✖ None</div>
        <div>✖ None</div>
        <div>
          <span className="highlight-cell">✔ 6 Services</span>
        </div>
      </div>
      <div className="compare-row">
        <div>
          <strong>Autonomous Self-Healing Loop</strong>
          <br />
          <small>Captures Docker errors, patches config, and re-tests</small>
        </div>
        <div>✖ No</div>
        <div>✖ No</div>
        <div>
          <span className="highlight-cell">✔ Bounded AI Healer</span>
        </div>
      </div>
      <div className="compare-row">
        <div>
          <strong>Runtime & HTTP Verification</strong>
          <br />
          <small>Checks container health + port probe + HTTP 200</small>
        </div>
        <div>✖ Exits</div>
        <div>✖ Exits</div>
        <div>
          <span className="highlight-cell">✔ Verified Live</span>
        </div>
      </div>
      <div className="compare-row">
        <div>
          <strong>Zero-Input Day-2 Management</strong>
          <br />
          <small>Persistent state for instant <code>yoink up</code> / <code>yoink stats</code></small>
        </div>
        <div>✖ None</div>
        <div>✖ None</div>
        <div>
          <span className="highlight-cell">✔ Built-in Lockfile</span>
        </div>
      </div>
      <div className="compare-row">
        <div>
          <strong>Offline & Sandboxed Execution</strong>
          <br />
          <small>Works without cloud with Ollama or <code>--no-agent</code></small>
        </div>
        <div>✔ Offline</div>
        <div>✔ Offline</div>
        <div>
          <span className="highlight-cell">✔ Ollama / --no-agent</span>
        </div>
      </div>
    </div>
  );
}
