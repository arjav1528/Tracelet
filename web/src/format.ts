function nanos(value: string): bigint | null {
  if (!/^\d+$/.test(value)) return null;
  try {
    return BigInt(value);
  } catch {
    return null;
  }
}

export function duration(value: string): string {
  const n = nanos(value);
  if (n === null) return "Unavailable";
  for (const [scale, unit] of [
    [1_000_000_000n, "s"],
    [1_000_000n, "ms"],
    [1_000n, "µs"],
  ] as const) {
    if (n >= scale) {
      const fraction = (((n % scale) * 1000n) / scale)
        .toString()
        .padStart(3, "0")
        .replace(/0+$/, "");
      return `${n / scale}${fraction ? "." + fraction : ""} ${unit}`;
    }
  }
  return `${n} ns`;
}

export function exactUTC(value: string): string {
  const n = nanos(value);
  if (n === null || n > 9_223_372_036_854_775_807n) return "Unavailable";
  // Only bounded whole milliseconds enter Date. Absolute nanoseconds never enter Number.
  const wholeMilliseconds = n / 1_000_000n;
  return (
    new Date(Number(wholeMilliseconds)).toISOString().slice(0, 19) +
    "." +
    (n % 1_000_000_000n).toString().padStart(9, "0") +
    "Z"
  );
}

export function localTime(value: string): string {
  const n = nanos(value);
  if (n === null || n > 9_223_372_036_854_775_807n) return "Unavailable";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(Number(n / 1_000_000n)));
}

export function bytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return "Unavailable";
  if (value >= 1024 ** 3)
    return `${(value / 1024 ** 3).toFixed(1).replace(/\.0$/, "")} GiB`;
  if (value >= 1024 ** 2)
    return `${(value / 1024 ** 2).toFixed(1).replace(/\.0$/, "")} MiB`;
  return `${Math.ceil(value / 1024)} KiB`;
}
