export interface DocSubSection {
  id: string;
  label: string;
}

export interface DocPageInfo {
  id: string;
  title: string;
  shortTitle: string;
  description: string;
  summary: string;
  toc: DocSubSection[];
}

export const DOC_PAGES: DocPageInfo[] = [
  {
    id: "getting-started",
    title: "Getting Started with Yoink",
    shortTitle: "Getting Started",
    summary: "Quickstart & Prerequisites",
    description: "Learn what Yoink does, the core 3-command workflow, system prerequisites, and runtime diagnostics.",
    toc: [
      { id: "overview", label: "Overview & Concept" },
      { id: "workflow", label: "3-Command Workflow" },
      { id: "prerequisites", label: "System Prerequisites" },
      { id: "doctor", label: "Runtime Diagnostics" }
    ]
  },
  {
    id: "installation",
    title: "Installation Methods & Source Build",
    shortTitle: "Installation",
    summary: "Script, Releases & Source",
    description: "Install via one-line curl installer, download precompiled GitHub releases, or clone and build from source.",
    toc: [
      { id: "install-script", label: "One-Line Shell Script" },
      { id: "install-releases", label: "Prebuilt GitHub Releases" },
      { id: "build-from-source", label: "Build Directly from GitHub" },
      { id: "makefile-targets", label: "Using the Makefile" }
    ]
  },
  {
    id: "configuration",
    title: "Configuration & Setup Wizard",
    shortTitle: "Configuration",
    summary: "LLM Providers & Keys",
    description: "Configure your LLM provider (OpenAI, Claude, Gemini, Groq, Ollama), PAT credentials, and inspect ~/.yoink/config.json.",
    toc: [
      { id: "setup-wizard", label: "Interactive Setup Wizard" },
      { id: "config-schema", label: "Config File Schema" },
      { id: "llm-providers", label: "Supported LLM Providers" },
      { id: "github-pat", label: "GitHub PAT Ingestion Security" }
    ]
  },
  {
    id: "commands",
    title: "Command Reference & Builder",
    shortTitle: "Command Reference",
    summary: "All 19 Commands & Flags",
    description: "Explore all 19 Yoink CLI commands, flags, and build custom commands visually with the interactive builder.",
    toc: [
      { id: "cmd-builder", label: "Interactive Command Builder" },
      { id: "cmd-init", label: "yoink init (Primary Entry)" },
      { id: "cmd-lifecycle", label: "Runtime Lifecycle (up/down/restart)" },
      { id: "cmd-observability", label: "Observability (status/logs/stats)" },
      { id: "cmd-env", label: "Environment Management (env/explain)" },
      { id: "cmd-maintenance", label: "Maintenance (doctor/update/incinerate)" }
    ]
  },
  {
    id: "architecture",
    title: "Architecture & Detection Engines",
    shortTitle: "Architecture & Engines",
    summary: "8-Stage Pipeline & Ingestion",
    description: "Deep dive into Yoink's 8 deterministic pipeline stages, 14 framework profiles, and 6 backing infrastructure engines.",
    toc: [
      { id: "arch-pipeline", label: "8-Stage Deterministic Pipeline" },
      { id: "detector-engine", label: "Framework Detector (14 Profiles)" },
      { id: "infra-inference", label: "Infrastructure Inference Rules" },
      { id: "env-intelligence", label: "5-Tier Environment Intelligence" }
    ]
  },
  {
    id: "healer",
    title: "Build Repair & SafeFS Security",
    shortTitle: "Healer & SafeFS",
    summary: "Autonomous Loop & Sandboxing",
    description: "How Yoink extracts root causes from failed Docker builds, applies bounded AI patches, and guarantees zero source tampering.",
    toc: [
      { id: "healer-loop", label: "6-Step Healing Loop" },
      { id: "root-cause-ranking", label: "High-Priority Error Ranking" },
      { id: "safefs-security", label: "SafeFS Security Guarantees" },
      { id: "persistent-state", label: "Persistent State Directory" }
    ]
  }
];
