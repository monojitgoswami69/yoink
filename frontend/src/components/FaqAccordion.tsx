"use client";

import React, { useState } from "react";

interface FaqItem {
  q: string;
  a: React.ReactNode;
}

const FAQS: FaqItem[] = [
  {
    q: "Do I need an LLM API key to use Yoink?",
    a: (
      <>
        <strong>No.</strong> You can run Yoink in 100% static mode using the <code>--no-agent</code> flag (<code>yoink init &lt;url&gt; --no-agent</code>), or configure a local, privacy-first <strong>Ollama</strong> instance during <code>yoink setup</code>. An API key (OpenAI, Anthropic, Gemini, Groq) is only needed if you want cloud-assisted build healing.
      </>
    )
  },
  {
    q: "How does Yoink handle private GitHub repositories?",
    a: (
      <>
        During <code>yoink setup</code>, you can supply a GitHub Personal Access Token (PAT). Yoink injects this token in-memory using <code>git -c http.extraheader=&quot;Authorization: Basic ...&quot;</code>. The token is <strong>never</strong> printed to standard output, never passed as a CLI argument, and is immediately stripped from git config once cloning finishes.
      </>
    )
  },
  {
    q: "Can the AI agent modify my source code?",
    a: (
      <>
        <strong>Never.</strong> Yoink enforces strict SafeFS sandboxing. The healing loop is only permitted to modify generated artifacts inside <code>yoink-outputs/</code> (such as <code>Dockerfile.*</code> and <code>docker-compose.yml</code>). Any attempt by the LLM to patch source files, traversals (<code>../</code>), or non-allowlisted filenames is strictly blocked.
      </>
    )
  },
  {
    q: "What happens if the build heal loop cannot fix a bug?",
    a: (
      <>
        The heal loop is capped by default to 3 attempts (configurable via <code>--heal-tries N</code>). If a stack cannot be fixed within budget, Yoink cleanly tears down broken containers, prints an honest, human-readable summary of the root cause, and exits with code <code>3 (BLOCKED)</code>.
      </>
    )
  },
  {
    q: "Where does Yoink store persistent state?",
    a: (
      <>
        Global configuration lives in <code>~/.yoink/config.json</code> (permissions chmod 0600). Project metadata, port allocations, and detection hashes are saved in <code>~/.yoink/state/&lt;project&gt;/yoink.lock</code>. You can remove a project anytime with <code>yoink incinerate &lt;project&gt;</code>.
      </>
    )
  }
];

export function FaqAccordion() {
  const [openIndex, setOpenIndex] = useState<number | null>(0);

  const toggle = (index: number) => {
    setOpenIndex(openIndex === index ? null : index);
  };

  return (
    <div className="accordion">
      {FAQS.map((faq, index) => {
        const isOpen = openIndex === index;
        return (
          <div key={index} className={`accordion-item ${isOpen ? "active" : ""}`}>
            <button
              className="accordion-header"
              onClick={() => toggle(index)}
              aria-expanded={isOpen}
            >
              <span>{faq.q}</span>
              <span className="accordion-icon" aria-hidden="true"></span>
            </button>
            <div className="accordion-body">
              <div className="accordion-content">{faq.a}</div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
