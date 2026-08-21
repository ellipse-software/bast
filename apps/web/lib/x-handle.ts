const HANDLE = /^[A-Za-z0-9_]{1,15}$/;

/** Strip @, trim, and lowercase. Returns null if it is not a valid X handle. */
export function normalizeXHandle(input: string): string | null {
  const trimmed = input.trim().replace(/^@+/, "");
  if (!HANDLE.test(trimmed)) return null;
  return trimmed.toLowerCase();
}
