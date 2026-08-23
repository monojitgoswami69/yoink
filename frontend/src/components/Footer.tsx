import React from "react";
import Link from "next/link";
import Image from "next/image";

export function Footer() {
  return (
    <footer style={{ backgroundColor: "var(--bg-card)", padding: "4rem 0 2.5rem", borderTop: "var(--border-lg)" }}>
      <div className="container">
        <div className="grid grid-4" style={{ gap: "2.5rem", marginBottom: "3rem" }}>
          <div>
            <Link href="/" className="brand" style={{ marginBottom: "1rem" }}>
              <Image
                src="/yoink-logo.png"
                alt="Yoink Logo"
                width={36}
                height={36}
                style={{ objectFit: "contain", display: "block" }}
              />
              <span>yoink</span>
            </Link>
            <p style={{ fontSize: "0.9rem", color: "var(--text-muted)" }}>
              Open-source CLI converting any repository into a verified running Docker environment.
            </p>
          </div>

          <div>
            <h4 style={{ fontSize: "0.95rem", textTransform: "uppercase", marginBottom: "1rem" }}>Product & Docs</h4>
            <ul style={{ listStyle: "none", display: "flex", flexDirection: "column", gap: "0.5rem", fontSize: "0.92rem", fontWeight: 700 }}>
              <li><Link href="/docs#getting-started" style={{ textDecoration: "underline" }}>Getting Started</Link></li>
              <li><Link href="/docs#installation" style={{ textDecoration: "underline" }}>Direct Source Build</Link></li>
              <li><Link href="/docs#commands" style={{ textDecoration: "underline" }}>CLI Commands Reference</Link></li>
              <li><Link href="/docs#architecture" style={{ textDecoration: "underline" }}>Architecture & Healer</Link></li>
            </ul>
          </div>

          <div>
            <h4 style={{ fontSize: "0.95rem", textTransform: "uppercase", marginBottom: "1rem" }}>Community & Source</h4>
            <ul style={{ listStyle: "none", display: "flex", flexDirection: "column", gap: "0.5rem", fontSize: "0.92rem", fontWeight: 700 }}>
              <li><a href="https://github.com/monojitgoswami69/yoink" target="_blank" rel="noopener noreferrer">GitHub Repository</a></li>
              <li><a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer">Releases & Binaries</a></li>
              <li><a href="https://github.com/monojitgoswami69/yoink/blob/main/install.sh" target="_blank" rel="noopener noreferrer">Install Script Source</a></li>
              <li><a href="https://github.com/monojitgoswami69/yoink/blob/main/LICENSE" target="_blank" rel="noopener noreferrer">MIT License</a></li>
            </ul>
          </div>

          <div>
            <h4 style={{ fontSize: "0.95rem", textTransform: "uppercase", marginBottom: "1rem" }}>Built With</h4>
            <p style={{ fontSize: "0.88rem", color: "var(--text-muted)" }}>
              Written in Go with Cobra, Bubble Tea & Lipgloss. Built with light-themed neobrutalism.
            </p>
          </div>
        </div>

        <div style={{ borderTop: "var(--border-sm)", paddingTop: "1.5rem", display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "center", gap: "1rem", fontSize: "0.85rem", color: "var(--text-muted)", fontWeight: 600 }}>
          <div>© {new Date().getFullYear()} Yoink Project. Open source under the MIT License.</div>
          <div>Adheres to Neobrutalism Design Guidelines.</div>
        </div>
      </div>
    </footer>
  );
}
