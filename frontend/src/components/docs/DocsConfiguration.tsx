import React from "react";
import { CodeBlock } from "@/components/CodeBlock";

export function DocsConfiguration() {
  return (
    <div>
      <section id="setup-wizard">
        <h2>Interactive Setup Wizard (<code>yoink setup</code>)</h2>
        <p>
          Yoink stores global credentials and preferences in <code>~/.yoink/config.json</code> with strict filesystem permissions (<code>chmod 0600</code>).
        </p>

        <CodeBlock
          code={`$ yoink setup\n? Select LLM Provider: Google Gemini\n? LLM Model [gemini-3.1-flash-lite]: gemini-3.1-flash-lite\n? LLM API Key: ••••••••••••••••••••••••••••••••\n? GitHub PAT (Optional, for private repos): ghp_••••••••••••••••••••\n\n✓ Configuration saved to ~/.yoink/config.json (chmod 0600)`}
          headerTitle="Interactive yoink setup"
        />
      </section>

      <section id="config-schema" style={{ marginTop: "3rem" }}>
        <h2>Config File Schema (<code>~/.yoink/config.json</code>)</h2>
        <p>You can edit or inspect the configuration file directly:</p>
        <CodeBlock
          code={`{\n  "llm_provider": "gemini",\n  "llm_model": "gemini-3.1-flash-lite",\n  "llm_api_key": "AIzaSy...",\n  "github_pat": "ghp_..."\n}`}
          headerTitle="JSON Schema"
        />
      </section>

      <section id="llm-providers" style={{ marginTop: "3rem" }}>
        <h2>Supported LLM Providers</h2>
        <div className="grid grid-2" style={{ gap: "1rem" }}>
          <div className="card card-static" style={{ padding: "1.25rem" }}>
            <div className="badge badge-yellow" style={{ marginBottom: "0.5rem" }}>Google Gemini</div>
            <p style={{ fontSize: "0.9rem", margin: 0 }}>Models: <code>gemini-3.1-flash-lite</code>, <code>gemini-2.0-flash</code>, <code>gemini-1.5-pro</code>.</p>
          </div>
          <div className="card card-static" style={{ padding: "1.25rem" }}>
            <div className="badge badge-green" style={{ marginBottom: "0.5rem" }}>Anthropic Claude</div>
            <p style={{ fontSize: "0.9rem", margin: 0 }}>Models: <code>claude-3-5-sonnet-latest</code>, <code>claude-3-5-haiku-latest</code>.</p>
          </div>
          <div className="card card-static" style={{ padding: "1.25rem" }}>
            <div className="badge badge-purple" style={{ marginBottom: "0.5rem" }}>OpenAI</div>
            <p style={{ fontSize: "0.9rem", margin: 0 }}>Models: <code>gpt-4o</code>, <code>gpt-4o-mini</code>, <code>o3-mini</code>.</p>
          </div>
          <div className="card card-static" style={{ padding: "1.25rem" }}>
            <div className="badge badge-cyan" style={{ marginBottom: "0.5rem" }}>Groq (Ultra-Fast)</div>
            <p style={{ fontSize: "0.9rem", margin: 0 }}>Models: <code>llama-3.3-70b-versatile</code>, <code>mixtral-8x7b-32768</code>.</p>
          </div>
          <div className="card card-static" style={{ padding: "1.25rem", gridColumn: "span 2" }}>
            <div className="badge badge-pink" style={{ marginBottom: "0.5rem" }}>Ollama (100% Local &amp; Offline)</div>
            <p style={{ fontSize: "0.9rem", margin: 0 }}>Runs against local endpoint <code>http://localhost:11434</code>. Zero data leaves your machine. No API key required.</p>
          </div>
        </div>
      </section>

      <section id="github-pat" style={{ marginTop: "3rem" }}>
        <h2>GitHub PAT Ingestion &amp; Transport Security</h2>
        <p>
          When cloning private repositories with a personal access token, Yoink uses Git&apos;s built-in <code>-c http.extraheader=&quot;Authorization: Basic &lt;base64&gt;&quot;</code> transport flag in-memory:
        </p>
        <ul>
          <li><strong>Zero Command-Line Leaks</strong>: The token is never passed as a CLI flag and never appears in <code>ps aux</code> process listings.</li>
          <li><strong>Zero Git Config Persistence</strong>: The token is never written to <code>.git/config</code>.</li>
          <li><strong>Automatic In-Memory Cleanup</strong>: The extraheader configuration is dropped as soon as cloning finishes.</li>
        </ul>
      </section>
    </div>
  );
}
