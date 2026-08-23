import React from "react";
import { CommandBuilder } from "@/components/CommandBuilder";

export function DocsCommands() {
  return (
    <div>
      <section id="cmd-builder">
        <h2>Interactive Command Builder</h2>
        <p>Visually configure flags, options, and arguments to generate the exact Yoink command for your workflow:</p>
        <CommandBuilder />
      </section>

      <section id="cmd-init" style={{ marginTop: "3rem" }}>
        <h2>1. <code>yoink init &lt;repo&gt;</code> (Primary Containerization Entry)</h2>
        <p>
          Clones the repository, detects framework and databases, synthesizes Dockerfiles + Compose, runs <code>docker compose build</code>, repairs errors, and validates HTTP reachability.
        </p>

        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Type</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--name &lt;name&gt;</code></td>
                <td>string</td>
                <td>Override the project identifier (defaults to repository name).</td>
              </tr>
              <tr>
                <td><code>--no-agent</code></td>
                <td>bool</td>
                <td>Run purely deterministic static analysis without LLM queries or API keys.</td>
              </tr>
              <tr>
                <td><code>--no-build</code></td>
                <td>bool</td>
                <td>Generate Docker configs and <code>yoink-outputs/</code> without invoking Docker build.</td>
              </tr>
              <tr>
                <td><code>--force</code></td>
                <td>bool</td>
                <td>Overwrite any existing output files and project state lockfile.</td>
              </tr>
              <tr>
                <td><code>--output &lt;dir&gt;</code></td>
                <td>string</td>
                <td>Specify target output directory for generated Docker assets (defaults to <code>yoink-outputs/</code>).</td>
              </tr>
              <tr>
                <td><code>--heal-tries &lt;N&gt;</code></td>
                <td>int</td>
                <td>Maximum build/heal retry loop iterations (default: <code>3</code>).</td>
              </tr>
              <tr>
                <td><code>--max-services &lt;N&gt;</code></td>
                <td>int</td>
                <td>Maximum services detected before warning (default: <code>10</code>).</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="cmd-lifecycle" style={{ marginTop: "3rem" }}>
        <h2>2. Runtime Lifecycle Commands</h2>
        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Command</th>
                <th>Aliases</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>yoink up [project]</code></td>
                <td><code>start</code></td>
                <td>Starts the project&apos;s Docker stack and blocks until container healthchecks pass.</td>
              </tr>
              <tr>
                <td><code>yoink down [project]</code></td>
                <td><code>stop</code></td>
                <td>Gracefully stops containers while preserving database persistent volumes.</td>
              </tr>
              <tr>
                <td><code>yoink restart [project]</code></td>
                <td>—</td>
                <td>Re-renders environment variables and restarts all project containers.</td>
              </tr>
              <tr>
                <td><code>yoink open [project]</code></td>
                <td>—</td>
                <td>Opens the primary application endpoint (e.g. <code>http://localhost:80/</code>) in your default browser.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="cmd-observability" style={{ marginTop: "3rem" }}>
        <h2>3. Observability &amp; Log Streaming</h2>
        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Command</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>yoink status [project]</code></td>
                <td>Displays container health status, assigned ports, and runtime uptime.</td>
              </tr>
              <tr>
                <td><code>yoink logs [project]</code></td>
                <td>Unified live multi-container log streaming with timestamp synchronization and color-coded service prefixes.</td>
              </tr>
              <tr>
                <td><code>yoink stats [project]</code></td>
                <td>Live terminal dashboard showing CPU percentage, RAM consumption, and network I/O per container.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="cmd-env" style={{ marginTop: "3rem" }}>
        <h2>4. Environment Variables &amp; Architecture Audit</h2>
        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Command</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>yoink env set &lt;KEY&gt;=&lt;VAL&gt;</code></td>
                <td>Overrides environment variable for a project without modifying compose files on disk.</td>
              </tr>
              <tr>
                <td><code>yoink env list [project]</code></td>
                <td>Prints all effective environment variables across detected services.</td>
              </tr>
              <tr>
                <td><code>yoink explain [project]</code></td>
                <td>Prints complete architectural audit: detected framework, inferred databases, port mapping, and volume configurations.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="cmd-maintenance" style={{ marginTop: "3rem" }}>
        <h2>5. Maintenance &amp; State Cleanup</h2>
        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Command</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>yoink doctor</code></td>
                <td>Comprehensive diagnostic check for Docker daemon, Compose v2, Git, and LLM API keys.</td>
              </tr>
              <tr>
                <td><code>yoink update</code></td>
                <td>Queries GitHub Releases, downloads latest binary, and self-updates in place.</td>
              </tr>
              <tr>
                <td><code>yoink incinerate &lt;project&gt;</code></td>
                <td>Tears down containers, removes associated Docker images, persistent volumes, outputs, and lockfile state.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
