"use client";

import React, { useState, useEffect } from "react";
import Link from "next/link";
import { ToastProvider } from "@/components/Toast";
import { Navbar } from "@/components/Navbar";
import { Footer } from "@/components/Footer";
import { DocsSidebar } from "@/components/DocsSidebar";
import { DocsToc } from "@/components/DocsToc";
import { DOC_PAGES, DocPageInfo } from "@/components/docs/docsData";
import { DocsOverview } from "@/components/docs/DocsOverview";
import { DocsInstallation } from "@/components/docs/DocsInstallation";
import { DocsConfiguration } from "@/components/docs/DocsConfiguration";
import { DocsCommands } from "@/components/docs/DocsCommands";
import { DocsArchitecture } from "@/components/docs/DocsArchitecture";
import { DocsHealer } from "@/components/docs/DocsHealer";

export default function DocsPage() {
  const [activePageId, setActivePageId] = useState("getting-started");

  // Read URL hash or param on mount
  useEffect(() => {
    const handleHash = () => {
      const hash = window.location.hash.replace("#", "");
      if (hash) {
        // Check if hash matches a page directly
        const pageMatch = DOC_PAGES.find((p) => p.id === hash);
        if (pageMatch) {
          setActivePageId(pageMatch.id);
          return;
        }
        // Check if hash matches a sub-heading in any page
        for (const p of DOC_PAGES) {
          if (p.toc.some((sub) => sub.id === hash)) {
            setActivePageId(p.id);
            setTimeout(() => {
              const el = document.getElementById(hash);
              if (el) el.scrollIntoView({ behavior: "smooth" });
            }, 100);
            return;
          }
        }
      }
    };

    handleHash();
    window.addEventListener("hashchange", handleHash);
    return () => window.removeEventListener("hashchange", handleHash);
  }, []);

  const handleSelectPage = (pageId: string, subSectionId?: string) => {
    setActivePageId(pageId);
    window.history.pushState(null, "", `#${subSectionId || pageId}`);
    if (subSectionId) {
      setTimeout(() => {
        const el = document.getElementById(subSectionId);
        if (el) {
          const header = document.querySelector(".site-header") as HTMLElement;
          const headerHeight = header ? header.offsetHeight : 72;
          const y = el.getBoundingClientRect().top + window.scrollY - headerHeight - 20;
          window.scrollTo({ top: Math.max(0, y), behavior: "smooth" });
        }
      }, 100);
    } else {
      window.scrollTo({ top: 0, behavior: "smooth" });
    }
  };

  const activePage: DocPageInfo =
    DOC_PAGES.find((p) => p.id === activePageId) || DOC_PAGES[0];
  const activeIndex = DOC_PAGES.findIndex((p) => p.id === activePage.id);
  const prevPage = activeIndex > 0 ? DOC_PAGES[activeIndex - 1] : null;
  const nextPage =
    activeIndex < DOC_PAGES.length - 1 ? DOC_PAGES[activeIndex + 1] : null;

  return (
    <ToastProvider>
      <Navbar isDocs={true} />

      <main className="container container-wide docs-wrapper" id="docs-main">
        {/* LEFT COLUMN: MAIN GUIDES & SECTIONS */}
        <DocsSidebar
          activePageId={activePage.id}
          onSelectPage={handleSelectPage}
        />

        {/* CENTER COLUMN: ACTIVE PAGE DOCUMENTATION */}
        <section className="docs-content">
          <article className="docs-article">
            {/* ACTIVE PAGE HEADER & BREADCRUMB */}
            <div style={{ marginBottom: "2.5rem", borderBottom: "var(--border-md)", paddingBottom: "1.5rem" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", fontSize: "0.85rem", fontWeight: 700, color: "var(--text-muted)", marginBottom: "0.75rem" }}>
                <Link href="/" style={{ textDecoration: "underline" }}>Yoink</Link>
                <span>/</span>
                <span>Docs</span>
                <span>/</span>
                <span style={{ color: "var(--text-main)", backgroundColor: "var(--neo-yellow-light)", padding: "0.1rem 0.4rem", borderRadius: "var(--radius-xs)", border: "1px solid #000" }}>
                  {activePage.shortTitle}
                </span>
              </div>
              <h1 style={{ fontSize: "clamp(1.65rem, 3.2vw, 2.25rem)", marginBottom: "0.75rem" }}>
                {activePage.title}
              </h1>
              <p style={{ fontSize: "1.15rem", color: "var(--text-muted)", margin: 0, lineHeight: 1.6 }}>
                {activePage.description}
              </p>
            </div>

            {/* ACTIVE PAGE CONTENT */}
            {activePage.id === "getting-started" && <DocsOverview />}
            {activePage.id === "installation" && <DocsInstallation />}
            {activePage.id === "configuration" && <DocsConfiguration />}
            {activePage.id === "commands" && <DocsCommands />}
            {activePage.id === "architecture" && <DocsArchitecture />}
            {activePage.id === "healer" && <DocsHealer />}

            {/* PREV / NEXT PAGE PAGINATION */}
            <div
              style={{
                marginTop: "4rem",
                paddingTop: "2rem",
                borderTop: "var(--border-md)",
                display: "grid",
                gridTemplateColumns: "1fr 1fr",
                gap: "1.5rem",
              }}
            >
              <div>
                {prevPage && (
                  <button
                    className="btn btn-white"
                    style={{ width: "100%", justifyContent: "flex-start", textAlign: "left", padding: "0.85rem 1.25rem", cursor: "pointer" }}
                    onClick={() => handleSelectPage(prevPage.id)}
                  >
                    <div>
                      <div style={{ fontSize: "0.75rem", color: "var(--text-muted)", textTransform: "uppercase" }}>← Previous</div>
                      <div style={{ fontSize: "1rem", fontWeight: 800 }}>{prevPage.shortTitle}</div>
                    </div>
                  </button>
                )}
              </div>
              <div style={{ textAlign: "right" }}>
                {nextPage && (
                  <button
                    className="btn btn-primary"
                    style={{ width: "100%", justifyContent: "flex-end", textAlign: "right", padding: "0.85rem 1.25rem", cursor: "pointer" }}
                    onClick={() => handleSelectPage(nextPage.id)}
                  >
                    <div>
                      <div style={{ fontSize: "0.75rem", color: "#000", textTransform: "uppercase" }}>Next →</div>
                      <div style={{ fontSize: "1rem", fontWeight: 800 }}>{nextPage.shortTitle}</div>
                    </div>
                  </button>
                )}
              </div>
            </div>
          </article>
        </section>

        {/* RIGHT COLUMN: ON THIS PAGE (SUBHEADINGS OF ACTIVE PAGE) */}
        <DocsToc activePage={activePage} />
      </main>

      <Footer />
    </ToastProvider>
  );
}
