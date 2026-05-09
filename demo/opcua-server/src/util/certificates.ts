import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { OPCUACertificateManager } from "node-opcua";

import { log } from "./logger.js";

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

  installBundledClientCertificate(pkiRoot);

  return { serverCertificateManager, userCertificateManager, pkiRoot };
}

// installBundledClientCertificate copies the repository-tracked demo
// client certificate from fixtures/client/ into the runtime user-trust
// store so cert-based auth works out of the box. Idempotent — if the
// cert is already present in trusted/certs the copy is a no-op.
//
// The accompanying private key is NOT copied to the server; clients
// hold it. Pair this with LOOPZE's central cert store (Source: file
// pointing at fixtures/client/client.{pem,key}) for an end-to-end
// reproducible setup.
function installBundledClientCertificate(pkiRoot: string): void {
  const fixture = resolve(projectRoot, "fixtures", "client", "client.pem");
  if (!existsSync(fixture)) {
    log.warn(`bundled client cert fixture not found at ${fixture}`);
    return;
  }
  const trustedDir = resolve(pkiRoot, "user", "trusted", "certs");
  mkdirSync(trustedDir, { recursive: true });
  const target = resolve(trustedDir, "loopze-demo-client.pem");
  copyFileSync(fixture, target);
  log.info(`bundled demo client cert installed → ${target}`);
}
