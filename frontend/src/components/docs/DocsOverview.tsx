import React from "react";
import { CodeBlock } from "@/components/CodeBlock";

export function DocsOverview() {
  return (
    <div>
      <section id="overview">
        <h2>What is Yoink?</h2>
        <p>
          <strong>Yoink</strong> is a single static Go CLI that takes any GitHub repository URL or local directory, discovers every deployable service, provisions required databases and caches, synthesizes optimized multi-stage Dockerfiles and <code>docker-compose.yml</code>, repairs build errors via an autonomous AI healing loop, and verifies live HTTP 200 reachability.
        </p>
      </section>

      <section id="workflow" style={{ marginTop: "2rem" }}>
        <h2>The 3-Command Basic Workflow</h2>
        <p>Get up and running with any repository in less than 60 seconds:</p>
        <CodeBlock
          code={`# 1. Configure your LLM provider (or select Ollama / --no-agent)\nyoink setup\n\n# 2. Ingest, generate, build, heal, and verify stack\nyoink init https://github.com/monojitgoswami69/certify\n\n# 3. Open the verified running application in your default browser\nyoink open`}
          headerTitle="3-Step Execution Workflow"
        />
      </section>

      <section id="prerequisites" style={{ marginTop: "2.5rem" }}>
        <h2>System Prerequisites</h2>
        <ul>
          <li><strong>Git</strong>: Required on your system <code>PATH</code> for cloning repositories.</li>
          <li><strong>Docker Engine + Docker Compose v2</strong>: Required at runtime to build and run container stacks (not required if running with <code>--no-build</code>).</li>
          <li><strong>Go 1.25+</strong>: Only required if building Yoink from source code.</li>
        </ul>
      </section>

      <section id="doctor" style={{ marginTop: "2.5rem" }}>
        <h2>Runtime Diagnostics (<code>yoink doctor</code>)</h2>
        <p>
          Run <code>yoink doctor</code> at any time to verify that your Docker daemon, Compose v2 plugin, Git, and LLM credentials are ready and configured properly:
        </p>
        <CodeBlock
          code={`$ yoink doctor\n[✓] Git binary detected on PATH (version 2.44.0)\n[✓] Docker daemon is running and responsive\n[✓] Docker Compose v2 plugin detected (v2.24.6)\n[✓] LLM Provider (Gemini) API key verified\n[✓] State directory permissions valid (~/.yoink chmod 0700)\n\nAll systems operational!`}
          headerTitle="yoink doctor output"
        />
      </section>
    </div>
  );
}
