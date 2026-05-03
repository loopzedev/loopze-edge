import { loadConfig } from "./util/config.js";
import { log, setLogLevel } from "./util/logger.js";
import { startDemoServer } from "./server.js";

async function main(): Promise<void> {
  const cfg = loadConfig();
  setLogLevel(cfg.logging.level);

  log.info("starting LOOPZE OPC UA Demo Server");
  log.info(
    `policies=${cfg.security.policies.join(",")} modes=${cfg.security.modes.join(",")} anon=${cfg.security.allowAnonymous}`,
  );

  const server = await startDemoServer(cfg);
  const endpoint = server.getEndpointUrl();
  log.info(`listening on ${endpoint}`);
  log.info(`primary endpoint: opc.tcp://${cfg.endpoint.hostname}:${cfg.endpoint.port}${cfg.endpoint.resourcePath}`);
  log.info(`endpoints advertised: ${server.endpoints[0]?.endpointDescriptions().length ?? 0}`);

  const shutdown = async (signal: string): Promise<void> => {
    log.info(`received ${signal}, shutting down`);
    try {
      await server.shutdown(1000);
      log.info("server stopped cleanly");
      process.exit(0);
    } catch (err) {
      log.error("error during shutdown", err);
      process.exit(1);
    }
  };
  process.on("SIGINT", () => void shutdown("SIGINT"));
  process.on("SIGTERM", () => void shutdown("SIGTERM"));
}

main().catch((err: unknown) => {
  log.error("fatal startup error", err);
  process.exit(1);
});
