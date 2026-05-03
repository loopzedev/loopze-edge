import { readFileSync } from "node:fs";
import { dirname, isAbsolute, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

export type SecurityPolicyName =
  | "None"
  | "Basic128Rsa15"
  | "Basic256"
  | "Basic256Sha256"
  | "Aes128_Sha256_RsaOaep"
  | "Aes256_Sha256_RsaPss";

export type SecurityModeName = "None" | "Sign" | "SignAndEncrypt";

export interface DemoUser {
  username: string;
  password: string;
  role: string;
  description?: string;
}

export interface DemoConfig {
  endpoint: {
    port: number;
    hostname: string;
    resourcePath: string;
    applicationName: string;
    applicationUri: string;
    productUri: string;
  };
  security: {
    policies: SecurityPolicyName[];
    modes: SecurityModeName[];
    allowAnonymous: boolean;
    autoAcceptUnknownCertificate: boolean;
  };
  users: DemoUser[];
  simulation: {
    tickIntervalMs: number;
    enableDynamicValues: boolean;
    motorTickMs: number;
  };
  logging: {
    level: "debug" | "info" | "warn" | "error";
    logSessions: boolean;
    logReads: boolean;
    logWrites: boolean;
    logSubscriptions: boolean;
  };
}

const here = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(here, "..", "..");

export function loadConfig(): DemoConfig {
  const configPath =
    process.env.LOOPZE_OPCUA_DEMO_CONFIG ??
    resolve(projectRoot, "config", "default.yaml");
  const raw = readFileSync(configPath, "utf8");
  const parsed = parseYaml(raw) as Record<string, unknown> & {
    users?: { configFile?: string };
  };

  const users = loadUsers(parsed.users?.configFile, configPath);
  return { ...(parsed as unknown as DemoConfig), users };
}

function loadUsers(filename: string | undefined, configPath: string): DemoUser[] {
  if (!filename) return [];
  const usersPath = isAbsolute(filename)
    ? filename
    : resolve(dirname(configPath), filename);
  const raw = readFileSync(usersPath, "utf8");
  const parsed = JSON.parse(raw) as { users?: DemoUser[] };
  return parsed.users ?? [];
}
