import React from "react";
import { CodeBlock } from "@/components/CodeBlock";

export function DocsInstallation() {
  return (
    <div>
      <section id="install-script">
        <h2>Option 1: Universal Shell Script Installer</h2>
        <p>
          The fastest way to install or update Yoink. It queries GitHub Releases for the latest version, matches your OS and CPU architecture, downloads the release archive, verifies permissions, and installs the binary to <code>~/.local/bin</code> (or <code>/usr/local/bin</code> if run as root).
        </p>

        <CodeBlock
          code="curl -sSfL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | bash"
          headerTitle="One-Line Install Command"
        />

        <h3>How <code>install.sh</code> Works Internally:</h3>
        <ol>
          <li><strong>OS &amp; Arch Detection</strong>: Runs <code>uname -s</code> and <code>uname -m</code> to detect Linux, macOS (Darwin), or Windows and <code>amd64</code> vs <code>arm64</code>.</li>
          <li><strong>Version Resolution</strong>: Queries <code>https://api.github.com/repos/monojitgoswami69/yoink/releases/latest</code> to obtain the active semantic release tag.</li>
          <li><strong>Release Binary Download</strong>: Downloads and unpacks the target tar.gz / zip archive into a temporary directory.</li>
          <li><strong>Fallback to Source Build</strong>: If prebuilt binaries are unavailable, <code>install.sh</code> automatically clones the repository and compiles via Go.</li>
          <li><strong>Shell PATH Assistant</strong>: Detects whether you are running <code>bash</code>, <code>zsh</code>, or <code>fish</code> and prints tailored <code>export PATH</code> instructions if needed.</li>
        </ol>
      </section>

      <section id="install-releases" style={{ marginTop: "3rem" }}>
        <h2>Option 2: Pre-Built Release Binaries</h2>
        <p>
          Download precompiled standalone binaries published by GoReleaser from <a href="https://github.com/monojitgoswami69/yoink/releases" target="_blank" rel="noopener noreferrer" style={{ textDecoration: "underline", fontWeight: 700 }}>GitHub Releases</a>:
        </p>

        <div className="cmd-table-wrapper">
          <table className="cmd-table">
            <thead>
              <tr>
                <th>Platform</th>
                <th>Architecture</th>
                <th>Archive Filename</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>macOS (Apple Silicon)</td>
                <td><code>arm64</code> (M1/M2/M3/M4)</td>
                <td><code>yoink_Darwin_arm64.tar.gz</code></td>
              </tr>
              <tr>
                <td>macOS (Intel)</td>
                <td><code>x86_64</code> / <code>amd64</code></td>
                <td><code>yoink_Darwin_x86_64.tar.gz</code></td>
              </tr>
              <tr>
                <td>Linux</td>
                <td><code>x86_64</code> / <code>amd64</code></td>
                <td><code>yoink_Linux_x86_64.tar.gz</code></td>
              </tr>
              <tr>
                <td>Linux</td>
                <td><code>arm64</code> / <code>aarch64</code></td>
                <td><code>yoink_Linux_arm64.tar.gz</code></td>
              </tr>
              <tr>
                <td>Windows</td>
                <td><code>x86_64</code></td>
                <td><code>yoink_Windows_x86_64.zip</code></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="build-from-source" style={{ marginTop: "3rem" }}>
        <h2>Option 3: Get Content Directly from GitHub &amp; Build Yourself</h2>
        <p>
          If you want bleeding-edge master commits, or prefer compiling binaries locally on your own machine, you can clone and build Yoink using the Go toolchain:
        </p>

        <CodeBlock
          code={`# 1. Clone the repository from GitHub\ngit clone https://github.com/monojitgoswami69/yoink.git\n\n# 2. Enter repository root\ncd yoink\n\n# 3. Build optimized static binary\ngo build -ldflags "-s -w" -o yoink .\n\n# 4. Move binary to your user PATH\nmkdir -p ~/.local/bin\nmv yoink ~/.local/bin/\n\n# 5. Verify installation\nyoink --version`}
          headerTitle="Manual Git Clone & Compilation"
        />
      </section>

      <section id="makefile-targets" style={{ marginTop: "2.5rem" }}>
        <h2>Using the Included Makefile</h2>
        <p>The repository includes a convenience <code>Makefile</code> with targets for building, testing, and cleaning:</p>

        <CodeBlock
          code={`make build          # Compiles ./yoink binary\nmake install        # Installs binary to $(INSTALL_DIR) or ~/.local/bin\nmake test           # Executes unit test suite (no network/docker required)\nmake vet            # Runs Go static code analysis\nmake clean          # Removes built artifacts`}
          headerTitle="Makefile Targets"
        />

        <h3>Running Directly Without Global Installation:</h3>
        <p>You can test any Yoink command without installing the binary globally using <code>go run</code>:</p>
        <CodeBlock
          code="go run . init https://github.com/tiangolo/full-stack-fastapi-template"
          headerTitle="go run . init"
        />
      </section>
    </div>
  );
}
