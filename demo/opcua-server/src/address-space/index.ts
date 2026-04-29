import type { AddressSpace, Namespace, UAObject } from "node-opcua";

import type { DemoConfig } from "../util/config.js";
import { log } from "../util/logger.js";
import { addArrays } from "./arrays.js";
import { addScalars } from "./scalars.js";
import { startSimulation } from "./simulation.js";
import { addStructures } from "./structures.js";
import { addWritables } from "./writables.js";

export interface DemoAddressSpace {
  namespace: Namespace;
  rootDemo: UAObject;
}

export async function buildAddressSpace(
  addressSpace: AddressSpace,
  cfg: DemoConfig,
): Promise<DemoAddressSpace> {
  const namespace = addressSpace.registerNamespace("urn:flint:demo");
  log.info(`registered namespace index=${namespace.index} uri='${namespace.namespaceUri}'`);

  const rootDemo = namespace.addFolder(addressSpace.rootFolder.objects, {
    browseName: "Demo",
    description: "Flint OPC UA demo data tree",
  });

  const staticFolder = namespace.addFolder(rootDemo, { browseName: "Static" });
  const dynamicFolder = namespace.addFolder(rootDemo, { browseName: "Dynamic" });
  const writableFolder = namespace.addFolder(rootDemo, { browseName: "Writable" });
  const structuresFolder = namespace.addFolder(rootDemo, { browseName: "Structures" });

  const scalars = addScalars(namespace, staticFolder, dynamicFolder);
  addArrays(namespace, staticFolder);
  const structures = await addStructures(addressSpace, namespace, structuresFolder);
  addWritables(addressSpace, namespace, writableFolder, structures);

  if (cfg.simulation.enableDynamicValues) {
    startSimulation(scalars, structures, cfg);
  }

  return { namespace, rootDemo };
}
