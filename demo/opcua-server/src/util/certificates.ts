import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { OPCUACertificateManager } from "node-opcua";

const here = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(here, "..", "..");

export interface CertManagers {
  serverCertificateManager: OPCUACertificateManager;
  userCertificateManager: OPCUACertificateManager;
  pkiRoot: string;
}

export async function setupCertificateManagers(
  autoAcceptUnknown: boolean,
): Promise<CertManagers> {
  const pkiRoot = resolve(projectRoot, "pki");
  mkdirSync(pkiRoot, { recursive: true });

  const serverCertificateManager = new OPCUACertificateManager({
    rootFolder: resolve(pkiRoot, "server"),
    automaticallyAcceptUnknownCertificate: autoAcceptUnknown,
    name: "server",
  });
  await serverCertificateManager.initialize();

  const userCertificateManager = new OPCUACertificateManager({
    rootFolder: resolve(pkiRoot, "user"),
    automaticallyAcceptUnknownCertificate: autoAcceptUnknown,
    name: "user",
  });
  await userCertificateManager.initialize();

  return { serverCertificateManager, userCertificateManager, pkiRoot };
}
