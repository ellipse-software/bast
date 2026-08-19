import { describe, expect, test } from "bun:test";

const installerPath = new URL("../public/install.ps1", import.meta.url);
const installerFile = Bun.file(installerPath);
const installer = await installerFile.text();

describe("Windows installer", () => {
  test("has no UTF-8 BOM so irm | iex can run on Windows PowerShell 5.1", async () => {
    const prefix = new Uint8Array(await installerFile.slice(0, 3).arrayBuffer());
    expect([...prefix]).not.toEqual([0xef, 0xbb, 0xbf]);
    expect(String.fromCharCode(prefix[0])).toBe("$");
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
});
