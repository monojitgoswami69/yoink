"use client";

import React, { useEffect, useState } from "react";
import { DocPageInfo } from "./docs/docsData";

interface DocsTocProps {
  activePage: DocPageInfo;
}

export function DocsToc({ activePage }: DocsTocProps) {
  const [activeId, setActiveId] = useState<string>("");

  useEffect(() => {
    if (activePage.toc.length > 0) {
      setActiveId(activePage.toc[0].id);
    }

    const handleScroll = () => {
      const header = document.querySelector(".site-header") as HTMLElement;
      const headerHeight = header ? header.offsetHeight : 72;
      const threshold = headerHeight + 28;

      const targets = activePage.toc.map((item) => item.id);
      let currentActive = activePage.toc[0]?.id || "";

      for (const id of targets) {
        const el = document.getElementById(id);
        if (el) {
          const rect = el.getBoundingClientRect();
          if (rect.top <= threshold) {
            currentActive = id;
          }
        }
      }

      setActiveId(currentActive);
    };

    window.addEventListener("scroll", handleScroll, { passive: true });
    handleScroll();
    return () => window.removeEventListener("scroll", handleScroll);
  }, [activePage]);

  const scrollToAnchor = (id: string, e: React.MouseEvent) => {
    e.preventDefault();
    const el = document.getElementById(id);
    if (el) {
      const header = document.querySelector(".site-header") as HTMLElement;
      const headerHeight = header ? header.offsetHeight : 72;
      const breathingSpace = 20;
      const y = el.getBoundingClientRect().top + window.scrollY - headerHeight - breathingSpace;
      window.scrollTo({ top: Math.max(0, y), behavior: "smooth" });
      window.history.pushState(null, "", `#${id}`);
      setActiveId(id);
    }
  };

  return (
    <aside className="docs-toc-wrapper">
      <div className="docs-toc-card">
        <div className="docs-toc-title">
          <span>ON THIS PAGE</span>
        </div>
        <ul className="docs-toc-list">
          {activePage.toc.map((item) => {
            const isActive = activeId === item.id;
            return (
              <li key={item.id}>
                <a
                  href={`#${item.id}`}
                  className={`docs-toc-link ${isActive ? "active" : ""}`}
                  onClick={(e) => scrollToAnchor(item.id, e)}
                >
                  {item.label}
                </a>
              </li>
            );
          })}
        </ul>
      </div>
    </aside>
  );
}

