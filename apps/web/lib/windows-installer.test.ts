import { describe, expect, test } from "bun:test";

const installerPath = new URL("../public/install.ps1", import.meta.url);
const installer = await Bun.file(installerPath).text();

describe("Windows installer", () => {
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
});
