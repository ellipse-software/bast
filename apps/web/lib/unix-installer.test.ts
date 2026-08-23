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
