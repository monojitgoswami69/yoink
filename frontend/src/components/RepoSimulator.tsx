"use client";

import React, { useState } from "react";
import { CodeBlock } from "./CodeBlock";

interface PresetData {
  url: string;
  name: string;
  frameworks: string[];
  infra: string[];
  healStory: string;
  composePreview: string;
}

const REPO_PRESETS: Record<string, PresetData> = {
  "fastapi-postgres": {
    url: "https://github.com/tiangolo/full-stack-fastapi-template",
    name: "full-stack-fastapi",
    frameworks: ["FastAPI (Python 3.12, Uvicorn)", "Vite / React (Node 20)"],
    infra: ["Postgres 16 Alpine (:5432)", "Redis 7 Alpine (:6379)"],
    healStory: "Python dependency conflict resolved; auto-bumped psycopg2-binary.",
    composePreview: `services:
  frontend:
    build:
      context: .
      dockerfile: yoink-outputs/Dockerfile.frontend
    ports:
      - "80:80"
    depends_on:
      backend:
        condition: service_healthy

  backend:
    build:
      context: .
      dockerfile: yoink-outputs/Dockerfile.backend
    ports:
      - "8000:8000"
    environment:
      DATABASE_URL: postgresql://yoink:yoink@postgres:5432/app
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: yoink
      POSTGRES_PASSWORD: yoink
      POSTGRES_DB: app
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U yoink"]

  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]`
  },
  "nextjs-redis": {
    url: "https://github.com/vercel/next.js/tree/canary/examples/with-redis",
    name: "next-redis-app",
    frameworks: ["Next.js 14 App Router (Standalone multi-stage)"],
    infra: ["Redis 7 Alpine (:6379)"],
    healStory: "Clean multi-stage Next standalone build with node_modules pruning.",
    composePreview: `services:
  web:
    build:
      context: .
      dockerfile: yoink-outputs/Dockerfile.web
    ports:
      - "3000:3000"
    environment:
      REDIS_URL: redis://redis:6379
      NODE_ENV: production
    depends_on:
      redis:
        condition: service_healthy

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]`
  },
  "express-mongo": {
    url: "https://github.com/expressjs/express-sample-app",
    name: "express-api",
    frameworks: ["Express.js (Node 20, pnpm)"],
    infra: ["MongoDB 7 (:27017)"],
    healStory: "Fixed missing start script in package.json by inferring entrypoint server.js.",
    composePreview: `services:
  api:
    build:
      context: .
      dockerfile: yoink-outputs/Dockerfile.api
    ports:
      - "5000:5000"
    environment:
      MONGO_URI: mongodb://mongo:27017/dev
    depends_on:
      mongo:
        condition: service_healthy

  mongo:
    image: mongo:7
    volumes:
      - mongodata:/data/db
    healthcheck:
      test: ["CMD", "mongosh", "--eval", "db.adminCommand('ping')"]`
  }
};

export function RepoSimulator() {
  const [activeKey, setActiveKey] = useState("fastapi-postgres");
  const [inputUrl, setInputUrl] = useState(REPO_PRESETS["fastapi-postgres"].url);
  const currentData = REPO_PRESETS[activeKey] || REPO_PRESETS["fastapi-postgres"];

  const handleSimulate = () => {
    const val = inputUrl.toLowerCase();
    if (val.includes("next")) {
      setActiveKey("nextjs-redis");
    } else if (val.includes("express") || val.includes("mongo")) {
      setActiveKey("express-mongo");
    } else {
      setActiveKey("fastapi-postgres");
    }
  };

  const handleSelectPreset = (key: string) => {
    setActiveKey(key);
    setInputUrl(REPO_PRESETS[key].url);
  };

  return (
    <div className="repo-sandbox">
      <div className="repo-presets">
        <span style={{ fontWeight: 800, fontSize: "0.85rem", textTransform: "uppercase" }}>Presets:</span>
        <button
          className={`preset-chip ${activeKey === "fastapi-postgres" ? "active" : ""}`}
          onClick={() => handleSelectPreset("fastapi-postgres")}
        >
          FastAPI + Postgres + Vite
        </button>
        <button
          className={`preset-chip ${activeKey === "nextjs-redis" ? "active" : ""}`}
          onClick={() => handleSelectPreset("nextjs-redis")}
        >
          Next.js 14 App Router + Redis
        </button>
        <button
          className={`preset-chip ${activeKey === "express-mongo" ? "active" : ""}`}
          onClick={() => handleSelectPreset("express-mongo")}
        >
          Express.js API + MongoDB
        </button>
      </div>

      <div className="repo-input-group">
        <input
          type="text"
          className="repo-input"
          value={inputUrl}
          onChange={(e) => setInputUrl(e.target.value)}
          placeholder="Enter any GitHub repo URL..."
        />
        <button className="btn btn-primary repo-inspect-btn" onClick={handleSimulate}>
          Simulate Yoink
        </button>
      </div>

      <div className="repo-sandbox-grid">
        <div className="card card-static" style={{ backgroundColor: "var(--bg-alt)", margin: 0, height: "100%" }}>
          <h4 style={{ marginBottom: "0.75rem" }}>Detected Stack & Inference</h4>
          <div style={{ marginBottom: "1rem" }}>
            <div style={{ fontSize: "0.85rem", fontWeight: 700, color: "var(--text-muted)", marginBottom: "0.35rem" }}>
              DEPLOYABLE SERVICES:
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.4rem" }}>
              {currentData.frameworks.map((f, i) => (
                <span key={i} className="badge badge-green">
                  {f}
                </span>
              ))}
            </div>
          </div>

          <div style={{ marginBottom: "1rem" }}>
            <div style={{ fontSize: "0.85rem", fontWeight: 700, color: "var(--text-muted)", marginBottom: "0.35rem" }}>
              INFERRED BACKING SERVICES:
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.4rem" }}>
              {currentData.infra.map((inf, i) => (
                <span key={i} className="badge badge-purple">
                  {inf}
                </span>
              ))}
            </div>
          </div>

          <div style={{ marginTop: "auto" }}>
            <div style={{ fontSize: "0.85rem", fontWeight: 700, color: "var(--text-muted)", marginBottom: "0.35rem" }}>
              HEAL ENGINE DECISION:
            </div>
            <p style={{ fontSize: "0.92rem", fontWeight: 600, color: "#000", margin: 0 }}>
              {currentData.healStory}
            </p>
          </div>
        </div>

        <CodeBlock
          code={currentData.composePreview}
          headerTitle="GENERATED docker-compose.yml"
          className="m-0"
        />
      </div>
    </div>
  );
}
