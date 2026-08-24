import { describe, expect, test } from "bun:test";

const installerPath = new URL("../public/install.ps1", import.meta.url);
const installerFile = Bun.file(installerPath);
const installer = await installerFile.text();

describe("Windows installer", () => {
  test("is UTF-8 BOM encoded for Windows PowerShell 5.1", async () => {
    const prefix = new Uint8Array(await installerFile.slice(0, 3).arrayBuffer());
    expect([...prefix]).toEqual([0xef, 0xbb, 0xbf]);
  });
  test("does not terminate the caller when Bast is current", () => {
    expect(installer).not.toMatch(/^\s*exit 0\s*$/m);
    expect(installer).toContain('Write-Success "Bast $version is already up to date."');
  });

  test("shows the same core installation feedback as the Unix installer", () => {
    expect(installer).toContain("██████╗  █████╗ ███████╗████████╗");
    expect(installer).toContain('Write-Info "Platform: windows/$goArchitecture"');
    expect(installer).toContain('Write-Step "Verifying SHA-256 checksum..."');
    expect(installer).toContain('Write-Success "Authenticode signature verified"');
    expect(installer).toContain("BAST_NO_TELEMETRY=1");
  });

  test("pins a stable GitHub release with BAST_VERSION", () => {
    expect(installer).toContain("releases/tags/$version");
    expect(installer).toContain('Write-Success "Requested version: $version"');
    expect(installer).toContain(
      "BAST_NIGHTLY_VERSION is for the nightly installer; set BAST_VERSION instead",
    );
  });
});
