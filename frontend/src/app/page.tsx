import React from "react";
import Link from "next/link";
import { ToastProvider } from "@/components/Toast";
import { Navbar } from "@/components/Navbar";
import { Footer } from "@/components/Footer";
import { CurlInstallBar } from "@/components/CurlInstallBar";
import { TerminalSimulator } from "@/components/TerminalSimulator";
import { ComparisonTable } from "@/components/ComparisonTable";
import { PipelineVisualizer } from "@/components/PipelineVisualizer";
import { FeatureBento } from "@/components/FeatureBento";
import { FaqAccordion } from "@/components/FaqAccordion";

export default function Home() {
  return (
    <ToastProvider>
      <Navbar />

      <main id="main">
        {/* HERO SECTION */}
        <section className="section" style={{ paddingTop: "3.5rem", paddingBottom: "2.5rem" }}>
          <div className="container">
            <div style={{ textAlign: "center", maxWidth: "860px", margin: "0 auto 2.5rem" }}>
              <h1 style={{ marginBottom: "1.25rem" }}>
                From GitHub URL to running Docker stack.<br />
                <span className="hl hl-green">One command.</span>
              </h1>

              <p style={{ fontSize: "1.25rem", color: "var(--text-muted)", lineHeight: 1.6, maxWidth: "780px", margin: "0 auto 2rem" }}>
                Yoink clones any repository, detects its services, provisions the databases it needs, generates production Dockerfiles, repairs build failures with an autonomous LLM heal loop, and verifies live HTTP health.
              </p>

              {/* PROMINENT CURL INSTALL COMMAND COPY AREA */}
              <CurlInstallBar />

              {/* Hero Action Buttons */}
              <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "center", gap: "1rem", marginTop: "1.5rem" }}>
                <Link className="btn btn-primary btn-lg" href="/docs#installation">Install Guide →</Link>
                <Link className="btn btn-white btn-lg" href="/docs">Read Documentation ↗</Link>
                <a className="btn btn-ghost btn-lg" href="#how">Explore 8-Step Pipeline ↓</a>
              </div>
            </div>

            {/* HERO INTERACTIVE TERMINAL SIMULATOR */}
            <TerminalSimulator />
          </div>
        </section>

        {/* SECTION: WHY YOINK (THE CHAIN, NOT A FILE) */}
        <section className="section" id="why" style={{ paddingTop: "2.5rem", paddingBottom: "4.5rem" }}>
          <div className="container">
            <div className="section-header" style={{ marginBottom: "2.5rem" }}>
              <h2 className="section-title">The barrier isn&apos;t a missing Dockerfile.<br />It&apos;s everything in between.</h2>
              <p className="section-subtitle">
                Most tools generate a generic template and walk away. Yoink solves the entire chain from git URL to addressable, verified local stack.
              </p>
            </div>

            <ComparisonTable />
          </div>
        </section>

        {/* SECTION: HOW IT WORKS (8-STAGE PIPELINE) */}
        <section className="section section-bordered" id="how">
          <div className="container">
            <div className="section-header">
              <h2 className="section-title">8 Deterministic Stages.<br />One Verified Outcome.</h2>
              <p className="section-subtitle">Click any stage below to inspect how Yoink analyzes, configures, and heals your repository.</p>
            </div>

            <PipelineVisualizer />
          </div>
        </section>

        {/* SECTION: FEATURE BENTO GRID */}
        <section className="section section-bordered" id="features">
          <div className="container">
            <div className="section-header">
              <h2 className="section-title">Built for Real-World Repositories</h2>
              <p className="section-subtitle">Every feature is designed to handle edge cases, missing configs, monorepos, and broken build outputs honestly.</p>
            </div>

            <FeatureBento />
          </div>
        </section>

        {/* SECTION: DAY-2 OPERATIONS */}
        <section className="section" id="day2">
          <div className="container">
            <div className="section-header">
              <h2 className="section-title">Manage Your Local Stack Without Leaving the Terminal</h2>
              <p className="section-subtitle">Yoink provides built-in lifecycle management, log streaming, environment overrides, and resource metrics.</p>
            </div>

            <div className="grid grid-3">
              <div className="card">
                <h4>yoink up &amp; down</h4>
                <p>Starts services and waits for healthchecks to turn green. Gracefully tears down containers while preserving persistent volumes.</p>
              </div>
              <div className="card">
                <h4>yoink logs &amp; stats</h4>
                <p>Unified live multi-container log streaming with timestamp synchronization and real-time CPU/memory metrics per container.</p>
              </div>
              <div className="card">
                <h4>yoink env &amp; explain</h4>
                <p>Override environment variables without editing compose files, and run <code>explain</code> for an instant architectural audit.</p>
              </div>
            </div>
          </div>
        </section>

        {/* SECTION: FAQ ACCORDION */}
        <section className="section section-bordered" id="faq">
          <div className="container container-narrow">
            <div className="section-header">
              <h2 className="section-title">Answers to Common Questions</h2>
              <p className="section-subtitle">Everything you need to know about safety, LLM keys, and local Docker management.</p>
            </div>

            <FaqAccordion />
          </div>
        </section>

        {/* FINAL CTA BAND */}
        <section className="section" style={{ backgroundColor: "var(--neo-yellow)", borderTop: "var(--border-xl)", borderBottom: "var(--border-xl)", textAlign: "center" }}>
          <div className="container container-narrow">
            <h2 style={{ fontSize: "clamp(1.75rem, 4vw, 2.75rem)", marginBottom: "1rem" }}>
              Ready to containerize any repository in seconds?
            </h2>
            <p style={{ fontSize: "1.25rem", fontWeight: 600, color: "#000", marginBottom: "2rem" }}>
              Copy the one-liner installer or dive into the complete documentation.
            </p>

            <CurlInstallBar />

            <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "center", gap: "1rem", marginTop: "1rem" }}>
              <Link className="btn btn-white btn-lg" href="/docs">Read Full Documentation →</Link>
              <a className="btn btn-white btn-lg" href="https://github.com/monojitgoswami69/yoink" target="_blank" rel="noopener noreferrer">★ Star on GitHub</a>
            </div>
          </div>
        </section>
      </main>

      <Footer />
    </ToastProvider>
  );
}
