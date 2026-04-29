type Level = "debug" | "info" | "warn" | "error";

const order: Record<Level, number> = { debug: 0, info: 1, warn: 2, error: 3 };

let currentLevel: Level = "info";

export function setLogLevel(level: Level): void {
  currentLevel = level;
}

function ts(): string {
  return new Date().toISOString();
}

function emit(level: Level, msg: string, ...rest: unknown[]): void {
  if (order[level] < order[currentLevel]) return;
  const line = `[${ts()}] ${level.toUpperCase().padEnd(5)} ${msg}`;
  if (level === "error") console.error(line, ...rest);
  else if (level === "warn") console.warn(line, ...rest);
  else console.log(line, ...rest);
}

export const log = {
  debug: (msg: string, ...rest: unknown[]) => emit("debug", msg, ...rest),
  info: (msg: string, ...rest: unknown[]) => emit("info", msg, ...rest),
  warn: (msg: string, ...rest: unknown[]) => emit("warn", msg, ...rest),
  error: (msg: string, ...rest: unknown[]) => emit("error", msg, ...rest),
};
