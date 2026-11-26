const tokenKey = "tracelet.token";
const validToken = (value: string) => /^[a-f0-9]{64}$/.test(value);

// The capability stays in this tab; URL fragments are removed before any request.
export function readToken(): string {
  const params = new URLSearchParams(window.location.hash.slice(1));
  if (params.has("token")) {
    const values = params.getAll("token");
    const token = values.length === 1 && validToken(values[0]) ? values[0] : "";
    window.history.replaceState(
      null,
      "",
      window.location.pathname + window.location.search,
    );
    try {
      if (token) sessionStorage.setItem(tokenKey, token);
      else sessionStorage.removeItem(tokenKey);
    } catch {
      /* The current tab can still use the in-memory capability. */
    }
    return token;
  }
  try {
    const token = sessionStorage.getItem(tokenKey) ?? "";
    return validToken(token) ? token : "";
  } catch {
    return "";
  }
}

export class APIError extends Error {
  constructor(public status: number) {
    super(
      status === 409
        ? "Trace data changed."
        : "The local request could not be completed.",
    );
  }
}

export async function apiGet<T>(
  path: string,
  token: string,
  signal: AbortSignal,
): Promise<T> {
  if (!path.startsWith("/api/v1/") || !validToken(token))
    throw new APIError(401);
  const response = await fetch(path, {
    method: "GET",
    credentials: "same-origin",
    cache: "no-store",
    redirect: "error",
    headers: { Accept: "application/json", "X-Tracelet-Token": token },
    signal,
  });
  if (!response.ok) throw new APIError(response.status);
  if (!response.headers.get("Content-Type")?.startsWith("application/json"))
    throw new APIError(502);
  return response.json() as Promise<T>;
}
