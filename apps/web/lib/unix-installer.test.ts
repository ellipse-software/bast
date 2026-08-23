import { describe, expect, test } from "bun:test";

const stable = await Bun.file(
  new URL("../public/install", import.meta.url),
).text();
const nightly = await Bun.file(
  new URL("../public/install-nightly", import.meta.url),
).text();

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
