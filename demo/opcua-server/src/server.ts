import {
  makeRoles,
  MessageSecurityMode,
  OPCUAServer,
  SecurityPolicy,
  type NodeId,
  type UserManagerOptions,
} from "node-opcua";

import { buildAddressSpace } from "./address-space/index.js";
import type {
  DemoConfig,
  DemoUser,
  SecurityModeName,
  SecurityPolicyName,
} from "./util/config.js";
import { setupCertificateManagers } from "./util/certificates.js";
import { log } from "./util/logger.js";

const POLICY_MAP: Record<SecurityPolicyName, SecurityPolicy> = {
  None: SecurityPolicy.None,
  Basic128Rsa15: SecurityPolicy.Basic128Rsa15,
  Basic256: SecurityPolicy.Basic256,
  Basic256Sha256: SecurityPolicy.Basic256Sha256,
  Aes128_Sha256_RsaOaep: SecurityPolicy.Aes128_Sha256_RsaOaep,
  Aes256_Sha256_RsaPss: SecurityPolicy.Aes256_Sha256_RsaPss,
};

const MODE_MAP: Record<SecurityModeName, MessageSecurityMode> = {
  None: MessageSecurityMode.None,
  Sign: MessageSecurityMode.Sign,
  SignAndEncrypt: MessageSecurityMode.SignAndEncrypt,
};

function buildUserManager(users: DemoUser[]): UserManagerOptions {
  const byName = new Map(users.map((u) => [u.username, u]));
  const manager = {
    isValidUser: (username: string, password: string): boolean => {
      const user = byName.get(username);
      const ok = !!user && user.password === password;
      log.info(
        `auth: username='${username}' role='${user?.role ?? "-"}' result=${
          ok ? "ok" : "denied"
        }`,
      );
      return ok;
    },
    // Newer node-opcua expects getUserRoles to return NodeId[] (a list
    // of WellKnownRole NodeIds), not the legacy semicolon-separated
    // string. makeRoles accepts the string form and turns it into the
    // expected NodeId[].
    getUserRoles: (username: string): NodeId[] => {
      const user = byName.get(username);
      switch (user?.role) {
        case "Admin":
          return makeRoles(
            "AuthenticatedUser;ConfigureAdmin;SecurityAdmin;Supervisor;Engineer;Operator",
          );
        case "Engineer":
          return makeRoles("AuthenticatedUser;Engineer;Operator");
        case "Operator":
          return makeRoles("AuthenticatedUser;Operator");
        default:
          return makeRoles("AuthenticatedUser");
      }
    },
  };
  return manager as unknown as UserManagerOptions;
}

export async function startDemoServer(cfg: DemoConfig): Promise<OPCUAServer> {
  const { serverCertificateManager, userCertificateManager, pkiRoot } =
    await setupCertificateManagers(cfg.security.autoAcceptUnknownCertificate);
  log.info(`PKI root: ${pkiRoot}`);

  const securityPolicies = cfg.security.policies.map((p) => POLICY_MAP[p]);
  const securityModes = cfg.security.modes.map((m) => MODE_MAP[m]);

  const server = new OPCUAServer({
    port: cfg.endpoint.port,
    hostname: cfg.endpoint.hostname,
    resourcePath: cfg.endpoint.resourcePath,
    buildInfo: {
      productName: cfg.endpoint.applicationName,
      buildNumber: "1",
      buildDate: new Date(),
    },
    serverInfo: {
      applicationName: { text: cfg.endpoint.applicationName, locale: "en" },
      applicationUri: cfg.endpoint.applicationUri,
      productUri: cfg.endpoint.productUri,
    },
    securityPolicies,
    securityModes,
    allowAnonymous: cfg.security.allowAnonymous,
    userManager: buildUserManager(cfg.users),
    serverCertificateManager,
    userCertificateManager,
    disableDiscovery: true,
  });

  await server.initialize();
  log.info("OPC UA server initialized");

  const addressSpace = server.engine.addressSpace;
  if (!addressSpace) throw new Error("address space not available after initialize()");
  await buildAddressSpace(addressSpace, cfg);

  attachSessionLogging(server, cfg);

  await server.start();
  return server;
}

function attachSessionLogging(server: OPCUAServer, cfg: DemoConfig): void {
  if (cfg.logging.logSessions) {
    server.on("create_session", (session) => {
      log.info(
        `session create: id=${session.nodeId.toString()} client='${session.clientDescription?.applicationName?.text ?? "?"}'`,
      );
    });
    server.on("session_closed", (session, deleteSubscriptions) => {
      log.info(
        `session close: id=${session.nodeId.toString()} deleteSubscriptions=${deleteSubscriptions}`,
      );
    });
  }
}
