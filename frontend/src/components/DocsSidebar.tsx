"use client";

import React, { useState } from "react";
import { DOC_PAGES, DocPageInfo } from "./docs/docsData";

interface DocsSidebarProps {
  activePageId: string;
  onSelectPage: (pageId: string, subSectionId?: string) => void;
}

export function DocsSidebar({ activePageId, onSelectPage }: DocsSidebarProps) {
  const [searchQuery, setSearchQuery] = useState("");

  const searchResults: { page: DocPageInfo; subId?: string; title: string; desc: string }[] = [];
  if (searchQuery.trim()) {
    const q = searchQuery.trim().toLowerCase();
    DOC_PAGES.forEach((page) => {
      if (
        page.title.toLowerCase().includes(q) ||
        page.shortTitle.toLowerCase().includes(q) ||
        page.description.toLowerCase().includes(q) ||
        page.summary.toLowerCase().includes(q)
      ) {
        searchResults.push({ page, title: page.title, desc: page.description });
      }
      page.toc.forEach((sub) => {
        if (sub.label.toLowerCase().includes(q)) {
          searchResults.push({
            page,
            subId: sub.id,
            title: `${page.shortTitle} → ${sub.label}`,
            desc: `Section inside ${page.title}`
          });
        }
      });
    });
  }

  return (
    <aside className="docs-sidebar">
      {/* SEARCH BOX */}
      <div className="docs-search-box">
        <svg
          className="docs-search-icon"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="11" cy="11" r="8"></circle>
          <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
        </svg>
        <input
          type="text"
          className="docs-search-input"
          placeholder="Search docs (e.g. build, init)..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          aria-label="Search documentation"
        />
        {searchQuery && (
          <button
            onClick={() => setSearchQuery("")}
            style={{
              position: "absolute",
              right: "0.75rem",
              top: "50%",
              transform: "translateY(-50%)",
              background: "none",
              border: "none",
              cursor: "pointer",
              fontWeight: 800,
              fontSize: "0.9rem"
            }}
            aria-label="Clear search"
          >
            ✕
          </button>
        )}
      </div>

      {/* SEARCH RESULTS OR MAIN PAGES */}
      {searchQuery.trim() ? (
        <div className="docs-nav-group">
          <div className="docs-nav-heading">Search Results ({searchResults.length})</div>
          {searchResults.length === 0 ? (
            <p style={{ fontSize: "0.85rem", color: "var(--text-muted)", padding: "0.5rem" }}>
              No matches found for &quot;{searchQuery}&quot;
            </p>
          ) : (
            <ul className="docs-nav-list">
              {searchResults.map((res, idx) => (
                <li key={idx}>
                  <button
                    className="docs-nav-link"
                    style={{ width: "100%", textAlign: "left", cursor: "pointer", background: "none" }}
                    onClick={() => {
                      onSelectPage(res.page.id, res.subId);
                      setSearchQuery("");
                    }}
                  >
                    <div style={{ fontWeight: 800, color: "var(--text-main)", fontSize: "0.88rem" }}>{res.title}</div>
                    <div style={{ fontSize: "0.78rem", color: "var(--text-muted)" }}>{res.desc}</div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="docs-nav-group">
          <div className="docs-nav-heading">Navigation</div>
          <ul className="docs-nav-list">
            {DOC_PAGES.map((page) => {
              const isActive = activePageId === page.id;
              return (
                <li key={page.id}>
                  <button
                    className={`docs-nav-link ${isActive ? "active" : ""}`}
                    style={{ width: "100%", textAlign: "left", cursor: "pointer" }}
                    onClick={() => onSelectPage(page.id)}
                    aria-current={isActive ? "page" : undefined}
                  >
                    {page.shortTitle}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </aside>
  );
}
