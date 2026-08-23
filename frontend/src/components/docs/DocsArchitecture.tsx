import React from "react";

export function DocsArchitecture() {
  return (
    <div>
      <section id="arch-pipeline">
        <h2>The 8-Stage Deterministic Pipeline</h2>
        <p>
          Unlike tools that dump generic Dockerfiles into your workspace and stop, Yoink orchestrates an end-to-end deterministic pipeline:
        </p>

        <ol>
          <li><strong>Ingest</strong>: Clones remote git repo into a shallow working directory or inspects local folder.</li>
          <li><strong>Detect Services</strong>: Traverses files, manifests, and package managers to identify application entrypoints.</li>
          <li><strong>Infer Infrastructure</strong>: Scans connection strings and dependencies to auto-provision Postgres, Redis, Mongo, MySQL, RabbitMQ.</li>
          <li><strong>Resolve Environment</strong>: Categorizes environment variables into 5 deterministic tiers.</li>
          <li><strong>Allocate Ports</strong>: Scans active host ports (5432, 6379, 3000, etc.) to assign conflict-free bindings.</li>
          <li><strong>Generate Docker Config</strong>: Synthesizes bespoke multi-stage Dockerfiles and <code>docker-compose.yml</code>.</li>
          <li><strong>Write Outputs &amp; Lock</strong>: Writes generated files to <code>yoink-outputs/</code> and stores metadata in <code>~/.yoink/state/</code>.</li>
          <li><strong>Build, Heal &amp; Verify</strong>: Executes Compose build, repairs failures via the AI loop, and checks live HTTP 200 health.</li>
        </ol>
      </section>

      <section id="detector-engine" style={{ marginTop: "3rem" }}>
        <h2>Framework Detector Engine (14 Profiles)</h2>
        <p>
          Yoink features built-in detection heuristics for 14 leading full-stack and backend frameworks:
        </p>

        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Framework</th>
                <th>Language</th>
                <th>Package Managers</th>
                <th>Default Exposed Port</th>
              </tr>
            </thead>
            <tbody>
              <tr><td>Next.js (App / Pages)</td><td>TypeScript / JS</td><td>pnpm, npm, yarn</td><td>3000</td></tr>
              <tr><td>Vite / React / Vue / Svelte</td><td>TypeScript / JS</td><td>pnpm, npm, yarn</td><td>80 (Nginx static)</td></tr>
              <tr><td>FastAPI</td><td>Python 3.10+</td><td>uv, poetry, pip</td><td>8000</td></tr>
              <tr><td>Django</td><td>Python 3.10+</td><td>poetry, pip, uv</td><td>8000</td></tr>
              <tr><td>NestJS</td><td>TypeScript</td><td>pnpm, npm, yarn</td><td>3000</td></tr>
              <tr><td>Express.js</td><td>TypeScript / JS</td><td>pnpm, npm, yarn</td><td>3000 / 5000</td></tr>
              <tr><td>Astro</td><td>TypeScript / JS</td><td>pnpm, npm, yarn</td><td>80 (Nginx) / 4321</td></tr>
              <tr><td>SvelteKit</td><td>TypeScript / JS</td><td>pnpm, npm, yarn</td><td>3000 / 80</td></tr>
              <tr><td>Flask</td><td>Python</td><td>pip, poetry, uv</td><td>5000</td></tr>
              <tr><td>Remix</td><td>TypeScript / JS</td><td>pnpm, npm, yarn</td><td>3000</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="infra-inference" style={{ marginTop: "3rem" }}>
        <h2>Infrastructure Inference Rules</h2>
        <p>Yoink matches environment variable names and code dependencies against infrastructure profiles:</p>
        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Hint / Pattern</th>
                <th>Inferred Service</th>
                <th>Docker Image</th>
                <th>Port</th>
              </tr>
            </thead>
            <tbody>
              <tr><td><code>DATABASE_URL</code>, <code>POSTGRES_*</code>, <code>PG*</code></td><td>Postgres</td><td><code>postgres:16-alpine</code></td><td>5432</td></tr>
              <tr><td><code>REDIS_URL</code>, <code>REDIS_*</code>, <code>CACHE_URL</code></td><td>Redis</td><td><code>redis:7-alpine</code></td><td>6379</td></tr>
              <tr><td><code>MONGO_URI</code>, <code>MONGODB_URL</code>, <code>MONGO_*</code></td><td>MongoDB</td><td><code>mongo:7</code></td><td>27017</td></tr>
              <tr><td><code>MYSQL_*</code>, <code>MYSQL_URL</code></td><td>MySQL</td><td><code>mysql:8</code></td><td>3306</td></tr>
              <tr><td><code>RABBITMQ_URL</code>, <code>AMQP_URL</code></td><td>RabbitMQ</td><td><code>rabbitmq:3-management</code></td><td>5672, 15672</td></tr>
              <tr><td><code>ELASTIC_URL</code>, <code>ELASTICSEARCH_URL</code></td><td>Elasticsearch</td><td><code>elasticsearch:8.13.4</code></td><td>9200</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="env-intelligence" style={{ marginTop: "3rem" }}>
        <h2>Environment Intelligence (5 Tiers)</h2>
        <p>Rather than treating every string matching <code>process.env</code> as a fatal requirement, Yoink classifies variables into 5 distinct tiers:</p>
        <ul>
          <li><strong>PROVIDED_DEFAULT</strong>: Supplied by repository templates (e.g. <code>.env.example</code>).</li>
          <li><strong>REQUIRED</strong>: Strong proof application fails to boot without it (e.g., Pydantic BaseSettings without default).</li>
          <li><strong>OPTIONAL</strong>: Fallback defaults exist in source code.</li>
          <li><strong>FEATURE_SPECIFIC</strong>: Accessed only by secondary routes or non-critical integrations.</li>
          <li><strong>UNKNOWN</strong>: Inconclusive static evidence.</li>
        </ul>
      </section>
    </div>
  );
}
