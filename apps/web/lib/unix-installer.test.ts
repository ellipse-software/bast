import { describe, expect, test } from "bun:test";

const stable = await Bun.file(new URL("../public/install", import.meta.url)).text();
const nightly = await Bun.file(
  new URL("../public/install-nightly", import.meta.url),
).text();

describe("Unix installer", () => {
  test("stable and nightly share completion install behavior", () => {
    for (const installer of [stable, nightly]) {
      expect(installer).toContain("BAST_NO_COMPLETIONS");
      expect(installer).toContain("# >>> bast completions >>>");
      expect(installer).toContain("install_shell_completions");
      expect(installer).toContain("Enabled shell completions");
      expect(installer).toContain("Open a new terminal, then press Tab after bast.");
      expect(installer).toMatch(
        /already up to date\."\n  install_shell_completions/,
      );
    }
  });

  test("uninstall instructions include completion files", () => {
    expect(stable).toContain("completion_dir");
    expect(stable).toContain("fish_completion");
    expect(nightly).toContain("completion_dir");
  });
});

describe("Unix stable installer", () => {
  test("pins a GitHub release with BAST_VERSION", () => {
    expect(stable).toContain('version="$BAST_VERSION"');
    expect(stable).toContain("is_stable_tag");
    expect(stable).toContain("Requested version:");
    expect(stable).toContain(
      "BAST_NIGHTLY_VERSION is for https://bast.sh/install-nightly",
    );
  });

  test("still resolves the latest release when BAST_VERSION is unset", () => {
    expect(stable).toContain("https://github.com/$repo/releases/latest");
    expect(stable).toContain("Latest release:");
  });
});

describe("Unix nightly installer", () => {
  test("pins a nightly tag with BAST_NIGHTLY_VERSION", () => {
    expect(nightly).toContain('version="$BAST_NIGHTLY_VERSION"');
    expect(nightly).toContain("is_nightly_tag");
    expect(nightly).toContain("Requested nightly:");
    expect(nightly).toContain(
      "BAST_VERSION is for https://bast.sh/install; set BAST_NIGHTLY_VERSION instead",
    );
  });
});

describe("Unix installer", () => {
  test("refuses a default user install over /usr/bin/bast", () => {
    for (const installer of [stable, nightly]) {
      expect(installer).toContain("refuse_system_package_conflict");
      expect(installer).toContain("/usr/bin/bast");
      expect(installer).toContain("BAST_INSTALL_DIR");
    }
  });

  test("still installs to ~/.local/bin by default", () => {
    expect(stable).toContain('install_dir="$HOME/.local/bin"');
    expect(nightly).toContain('install_dir="$HOME/.local/bin"');
  });
});
