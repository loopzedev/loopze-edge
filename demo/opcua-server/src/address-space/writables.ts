import {
  type AddressSpace,
  DataType,
  type Namespace,
  StatusCodes,
  type UAObject,
  Variant,
} from "node-opcua";

import { log } from "../util/logger.js";
import type { DemoStructures } from "./structures.js";

export function addWritables(
  addressSpace: AddressSpace,
  ns: Namespace,
  writableFolder: UAObject,
  structures: DemoStructures,
): void {
  // ── Setpoint (Double) ─────────────────────────────────────────────
  let setpoint = 0;
  ns.addVariable({
    organizedBy: writableFolder,
    browseName: "Setpoint",
    nodeId: "s=Demo.Writable.Setpoint",
    dataType: DataType.Double,
    accessLevel: "CurrentRead | CurrentWrite",
    userAccessLevel: "CurrentRead | CurrentWrite",
    minimumSamplingInterval: 1000,
    value: {
      get: () => new Variant({ dataType: DataType.Double, value: setpoint }),
      set: (variant: Variant) => {
        const v = Number(variant.value);
        if (!Number.isFinite(v)) return StatusCodes.BadOutOfRange;
        log.info(`write Setpoint: ${setpoint} -> ${v}`);
        setpoint = v;
        return StatusCodes.Good;
      },
    },
  });

  // ── Mode (Int32 with EnumDefinition) ──────────────────────────────
  // 0 = Manual, 1 = Auto, 2 = Service
  let mode = 1;
  ns.addVariable({
    organizedBy: writableFolder,
    browseName: "Mode",
    nodeId: "s=Demo.Writable.Mode",
    dataType: DataType.Int32,
    accessLevel: "CurrentRead | CurrentWrite",
    userAccessLevel: "CurrentRead | CurrentWrite",
    description: "0=Manual 1=Auto 2=Service",
    minimumSamplingInterval: 1000,
    value: {
      get: () => new Variant({ dataType: DataType.Int32, value: mode }),
      set: (variant: Variant) => {
        const v = Number(variant.value);
        if (!Number.isInteger(v) || v < 0 || v > 2) return StatusCodes.BadOutOfRange;
        log.info(`write Mode: ${mode} -> ${v}`);
        mode = v;
        return StatusCodes.Good;
      },
    },
  });

  // ── Command (String) ─────────────────────────────────────────────
  let command = "idle";
  ns.addVariable({
    organizedBy: writableFolder,
    browseName: "Command",
    nodeId: "s=Demo.Writable.Command",
    dataType: DataType.String,
    accessLevel: "CurrentRead | CurrentWrite",
    userAccessLevel: "CurrentRead | CurrentWrite",
    minimumSamplingInterval: 1000,
    value: {
      get: () => new Variant({ dataType: DataType.String, value: command }),
      set: (variant: Variant) => {
        const v = String(variant.value ?? "");
        log.info(`write Command: '${command}' -> '${v}'`);
        command = v;
        return StatusCodes.Good;
      },
    },
  });

  // ── MotorCmd (ExtensionObject of MotorCommandType) ───────────────
  let lastCmd = {
    TargetSpeed: 0,
    AccelRamp: 100,
    Direction: "CW",
    EnableLimits: true,
  };
  ns.addVariable({
    organizedBy: writableFolder,
    browseName: "MotorCmd",
    nodeId: "s=Demo.Writable.MotorCmd",
    dataType: structures.motorCommandType.nodeId,
    accessLevel: "CurrentRead | CurrentWrite",
    userAccessLevel: "CurrentRead | CurrentWrite",
    minimumSamplingInterval: 1000,
    value: {
      get: () =>
        new Variant({
          dataType: DataType.ExtensionObject,
          value: addressSpace.constructExtensionObject(
            structures.motorCommandType,
            lastCmd,
          ),
        }),
      set: (variant: Variant) => {
        const ext = variant.value as Record<string, unknown> | null;
        if (!ext || typeof ext !== "object") return StatusCodes.BadTypeMismatch;
        lastCmd = {
          TargetSpeed: Number(ext.TargetSpeed ?? lastCmd.TargetSpeed),
          AccelRamp: Number(ext.AccelRamp ?? lastCmd.AccelRamp),
          Direction: String(ext.Direction ?? lastCmd.Direction),
          EnableLimits: Boolean(ext.EnableLimits ?? lastCmd.EnableLimits),
        };
        log.info(
          `write MotorCmd: TargetSpeed=${lastCmd.TargetSpeed} Direction=${lastCmd.Direction}`,
        );
        return StatusCodes.Good;
      },
    },
  });
}
