import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Yoink — Turn Any GitHub Repo into a Verified Running Docker Stack",
  description: "Yoink is an open-source Go CLI that clones a repository, detects services, provisions backing infrastructure, generates Docker configs, repairs broken builds with an LLM heal loop, and verifies live HTTP reachability.",
  icons: {
    icon: "/yoink-logo.png",
    apple: "/yoink-logo.png",
  },
  openGraph: {
    title: "Yoink: Turn Any GitHub Repo into a Verified Running Docker Stack",
    description: "Clone, detect, provision, generate, build, heal, run. One command, no Docker expertise required. Open source, MIT.",
    url: "https://github.com/monojitgoswami69/yoink",
    siteName: "Yoink CLI",
    type: "website",
  },
  twitter: {
    card: "summary",
    title: "Yoink: Turn Any GitHub Repo into a Verified Running Docker Stack",
    description: "One command from GitHub URL to a verified, running local Docker stack.",
  }
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,700;12..96,800;12..96,900&family=Inter:wght@400;500;600;700;800&family=JetBrains+Mono:ital,wght@0,400;0,600;0,700;0,800;1,400&family=Plus+Jakarta+Sans:wght@700;800;900&display=swap"
          rel="stylesheet"
        />
      </head>
      <body>
        <a className="skip-link" href="#main">Skip to main content</a>
        {children}
      </body>
    </html>
  );
}
