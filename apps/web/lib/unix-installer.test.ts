import { describe, expect, test } from "bun:test";

const stable = await Bun.file(new URL("../public/install", import.meta.url)).text();
const nightly = await Bun.file(
  new URL("../public/install-nightly", import.meta.url),
).text();

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
