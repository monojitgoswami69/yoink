"use client";

import React, { useState } from "react";

const PIPELINE_STAGES = [
  {
    id: 1,
    title: "1. Clone Repository",
    tag: "GIT INGESTION",
    desc: "Shallow clone of repository with private token isolation.",
    text: "Yoink performs an optimized shallow clone of the target repository. If a GitHub Personal Access Token is configured, it is injected safely via git -c http.extraheader so secrets never leak into command line arguments or .git/config.",
    points: [
      "Secure in-memory PAT injection",
      "Branch & subfolder tree targeting support",
      "Automatic cleanup of authorization headers"
    ],
    output: `[1/8] Clone repository
→ Git clone --depth=1 https://github.com/...
✓ Ingested 142 repository files (size: 4.8MB)
✓ Stripped transient auth headers from git transport`
  },
  {
    id: 2,
    title: "2. Generate Repo Tree",
    tag: "ANALYSIS",
    desc: "Safe filtered filesystem traversal skipping build noise.",
    text: "Constructs an in-memory structural tree of the repository while ignoring heavy build and vendor directories (such as node_modules, .venv, .git, dist, build).",
    points: [
      "Ignores monorepo noise (examples, docs, tests)",
      "Detects package boundary roots and workspaces",
      "Prepares hierarchical manifest map for detectors"
    ],
    output: `[2/8] Generate repository tree
├── frontend/ (package.json, vite.config.ts)
├── backend/  (pyproject.toml, app/main.py)
└── docker-compose.yml (not found, generating...)
✓ Tree indexed in 18ms`
  },
  {
    id: 3,
    title: "3. Detect Services",
    tag: "HEURISTICS & AGENT",
    desc: "Identifies 14 framework profiles and package managers.",
    text: "Analyzes package manifests, configurations, and directory topologies to identify deployable services (Next.js, FastAPI, Vite, Express, NestJS, Django, etc.), start commands, and port allocations.",
    points: [
      "14 Framework profiles with exact start commands",
      "Package manager inference (npm, pnpm, yarn, uv, poetry, pip)",
      "Bounded LLM validation for edge cases and multi-stage setups"
    ],
    output: `[3/8] Detect services
  • service-1: TypeScript / Vite SPA (pnpm, port 80/nginx)
  • service-2: Python 3.12 / FastAPI (uvicorn, port 8000)
✓ High confidence detection score (0.98)`
  },
  {
    id: 4,
    title: "4. Extract Env Vars",
    tag: "INTELLIGENCE",
    desc: "5-Tier environment variable discovery & classification.",
    text: "Scans repository code and templates without blind guessing. Classifies discovered variables into PROVIDED_DEFAULT, REQUIRED, OPTIONAL, FEATURE_SPECIFIC, or UNKNOWN.",
    points: [
      "Preserves repository .env.example templates",
      "Identifies unhandled missing environment crashes",
      "Marks credential placeholders instead of dummy values"
    ],
    output: `[4/8] Extract environment vars
  • DATABASE_URL (inferred from backend/database.py)
  • REDIS_HOST   (inferred from app/cache.py)
  • API_SECRET   (marked as REQUIRED placeholder)
✓ Generated env-vars/service-*/.env templates`
  },
  {
    id: 5,
    title: "5. Infer Backing Services",
    tag: "INFRASTRUCTURE",
    desc: "Automatic local container provisioning for databases & queues.",
    text: "Examines detected environment variables and dependency manifests. Automatically provisions healthy local containers (Postgres, MySQL, Redis, Mongo, RabbitMQ, Elasticsearch) with named persistent volumes.",
    points: [
      "Auto-provisions Postgres, Redis, Mongo, RabbitMQ, etc.",
      "Wires healthchecks and depends_on: service_healthy",
      "Bypasses local provisioning if cloud SDKs (Neon, Upstash) are found"
    ],
    output: `[5/8] Infer backing services
  • postgres:16-alpine (linked to backend, healthcheck: pg_isready)
  • redis:7-alpine     (linked to backend, healthcheck: redis-cli ping)
✓ Generated network: yoink-network-bridge`
  },
  {
    id: 6,
    title: "6. Generate Docker Config",
    tag: "GENERATION",
    desc: "Creates multi-stage Dockerfiles and docker-compose.yml.",
    text: "Synthesizes optimized production-grade Dockerfiles tailored specifically to each framework (multi-stage builds for Next.js, Nginx static servers for Vite, lean alpine containers for Python) and unites them in docker-compose.yml.",
    points: [
      "Framework-tailored multi-stage compilation",
      "Monorepo-aware context rooting",
      "Automatic host port conflict resolution"
    ],
    output: `[6/8] Generate Docker config
  → yoink-outputs/Dockerfile.service-1 (Vite + Nginx)
  → yoink-outputs/Dockerfile.service-2 (FastAPI + Python 3.12)
  → yoink-outputs/docker-compose.yml`
  },
  {
    id: 7,
    title: "7. Write Outputs",
    tag: "PERSISTENCE",
    desc: "Writes isolated outputs and writes immutable state lockfile.",
    text: "Writes all generated assets cleanly into yoink-outputs/ without polluting repository source files, and records detection metadata into ~/.yoink/state/<project>/yoink.lock for zero-input Day-2 operations.",
    points: [
      "Isolated yoink-outputs/ structure",
      "Writes .dockerignore to optimize build context",
      "Saves authoritative yoink.lock state file"
    ],
    output: `[7/8] Write outputs
✓ Created yoink-outputs/quick_start.md
✓ Initialized state in ~/.yoink/state/certify/
✓ State locked with schema v1.2`
  },
  {
    id: 8,
    title: "8. Build & Self-Heal Loop",
    tag: "AUTONOMOUS REPAIR",
    desc: "Executes Compose build with bounded LLM repair & HTTP verification.",
    text: "Runs docker compose build. If compilation or dependency errors arise, Yoink extracts the root cause, applies deterministic fixes or bounded LLM patches to generated Docker configs, rebuilds, and verifies live HTTP reachability.",
    points: [
      "Prioritizes root causes (TypeScript, npm, Python errors)",
      "Strict SafeFS sandboxing: agent can only patch generated files",
      "Independent verification: checks container status, port probe & HTTP 200"
    ],
    output: `[8/8] Build & heal
→ docker compose build
⚠ TypeScript TS2307: Cannot find module '@types/node'
→ Bounded Agent repair [Attempt 1/3]:
  Patched Dockerfile.service-1: Added @types/node devDependency
→ docker compose up -d (waiting for healthchecks...)
✓ Container service-1: HEALTHY
✓ Container service-2: HEALTHY
✓ HTTP GET http://localhost:80/ -> 200 OK`
  }
];

export function PipelineVisualizer() {
  return (
    <div className="pipeline-wrapper">
      <div className="pipeline-steps">
        {PIPELINE_STAGES.map((stage) => (
          <div
            key={stage.id}
            className="pipeline-step-card"
          >
            <span className="step-card-num">{stage.id}</span>
            <div className="step-card-title">{stage.title.split(". ")[1]}</div>
            <div className="step-card-desc">{stage.desc}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
