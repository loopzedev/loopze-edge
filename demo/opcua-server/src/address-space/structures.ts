import {
  type AddressSpace,
  DataType,
  ensureDatatypeExtracted,
  type ExtensionObject,
  type Namespace,
  resolveNodeId,
  type UADataType,
  type UAObject,
  type UAVariable,
  Variant,
} from "node-opcua";
import type { StructureFieldOptions } from "node-opcua-types";

export interface DemoStructures {
  motorStatusType: UADataType;
  motorStatusVar: UAVariable;
  sensorReadingType: UADataType;
  sensorReadingVar: UAVariable;
  motorCommandType: UADataType;
  recipeType: UADataType;
  recipeStepType: UADataType;
  recipeVar: UAVariable;
  // Mutable state used by simulation tick:
  currentMotor: { Speed: number; Torque: number; FaultCode: number; Running: boolean; Mode: string };
}

interface FieldSpec {
  name: string;
  dataType: DataType | string; // built-in DataType or NodeId string for custom types
  valueRank?: number;
  description?: string;
}

function defineStructure(
  ns: Namespace,
  browseName: string,
  fields: FieldSpec[],
): UADataType {
  const partialDefinition: StructureFieldOptions[] = fields.map((f) => ({
    name: f.name,
    dataType:
      typeof f.dataType === "string"
        ? resolveNodeId(f.dataType)
        : resolveNodeId(DataType[f.dataType]),
    valueRank: f.valueRank ?? -1,
    description: f.description,
  }));

  const dt = ns.createDataType({
    browseName,
    isAbstract: false,
    subtypeOf: "Structure",
    partialDefinition,
  });

  // Attach a "Default Binary" encoding object so the wire-encoder can
  // serialize ExtensionObject values of this type. Without this, reads of
  // structured variables fail with "Cannot find encodingDefaultBinary".
  ns.addObject({
    browseName: "Default Binary",
    typeDefinition: resolveNodeId("DataTypeEncodingType"), // ns=0;i=76
    references: [
      {
        referenceType: "HasEncoding",
        isForward: false,
        nodeId: dt.nodeId,
      },
    ],
  });

  return dt;
}

function buildExtensionObject(
  addressSpace: AddressSpace,
  dataType: UADataType,
  value: Record<string, unknown>,
): ExtensionObject {
  return addressSpace.constructExtensionObject(dataType, value);
}

export async function addStructures(
  addressSpace: AddressSpace,
  ns: Namespace,
  structuresFolder: UAObject,
): Promise<DemoStructures> {
  // ── Type definitions ─────────────────────────────────────────────
  const motorStatusType = defineStructure(ns, "MotorStatusType", [
    { name: "Speed", dataType: DataType.Double, description: "RPM" },
    { name: "Torque", dataType: DataType.Double, description: "Nm" },
    { name: "FaultCode", dataType: DataType.UInt32 },
    { name: "Running", dataType: DataType.Boolean },
    { name: "Mode", dataType: DataType.String, description: "Auto | Manual | Service" },
  ]);

  const sensorReadingType = defineStructure(ns, "SensorReadingType", [
    { name: "Value", dataType: DataType.Double },
    { name: "Unit", dataType: DataType.String },
    { name: "Quality", dataType: DataType.UInt16 },
    { name: "Timestamp", dataType: DataType.DateTime },
  ]);

  const motorCommandType = defineStructure(ns, "MotorCommandType", [
    { name: "TargetSpeed", dataType: DataType.Double },
    { name: "AccelRamp", dataType: DataType.UInt32 },
    { name: "Direction", dataType: DataType.String, description: "CW | CCW" },
    { name: "EnableLimits", dataType: DataType.Boolean },
  ]);

  const recipeStepType = defineStructure(ns, "RecipeStepType", [
    { name: "StepNo", dataType: DataType.UInt16 },
    { name: "Duration", dataType: DataType.Double, description: "seconds" },
    { name: "Setpoint", dataType: DataType.Double },
  ]);

  // RecipeType has an array of RecipeStepType (referenced by NodeID string)
  const recipeType = defineStructure(ns, "RecipeType", [
    { name: "Name", dataType: DataType.String },
    { name: "Version", dataType: DataType.UInt32 },
    { name: "Steps", dataType: recipeStepType.nodeId.toString(), valueRank: 1 },
  ]);

  // Register the per-namespace data type factory so constructExtensionObject()
  // can find encoders/decoders for our custom UDTs. node-opcua caches the
  // ExtraDataTypeManager on the address space; if it was already populated
  // (e.g. by server.initialize()) before our namespace existed, the cached
  // version won't know about us. Clear it so the next call rebuilds with the
  // current namespace array.
  (addressSpace as unknown as { $$extraDataTypeManager?: unknown }).$$extraDataTypeManager =
    undefined;
  await ensureDatatypeExtracted(addressSpace);

  // ── Instances ─────────────────────────────────────────────
  const currentMotor = {
    Speed: 1450.5,
    Torque: 12.3,
    FaultCode: 0,
    Running: true,
    Mode: "Auto",
  };

  const motorStatusVar = ns.addVariable({
    organizedBy: structuresFolder,
    browseName: "MotorStatus",
    nodeId: "s=Demo.Structures.MotorStatus",
    dataType: motorStatusType.nodeId,
    minimumSamplingInterval: 100,
    value: {
      get: () =>
        new Variant({
          dataType: DataType.ExtensionObject,
          value: buildExtensionObject(addressSpace, motorStatusType, currentMotor),
        }),
    },
  });

  const sensorReadingVar = ns.addVariable({
    organizedBy: structuresFolder,
    browseName: "SensorReading",
    nodeId: "s=Demo.Structures.SensorReading",
    dataType: sensorReadingType.nodeId,
    minimumSamplingInterval: 1000,
    value: {
      get: () =>
        new Variant({
          dataType: DataType.ExtensionObject,
          value: buildExtensionObject(addressSpace, sensorReadingType, {
            Value: 23.4,
            Unit: "degC",
            Quality: 192,
            Timestamp: new Date(),
          }),
        }),
    },
  });

  const recipeSteps = [
    buildExtensionObject(addressSpace, recipeStepType, {
      StepNo: 1,
      Duration: 5.0,
      Setpoint: 100.0,
    }),
    buildExtensionObject(addressSpace, recipeStepType, {
      StepNo: 2,
      Duration: 10.0,
      Setpoint: 250.0,
    }),
    buildExtensionObject(addressSpace, recipeStepType, {
      StepNo: 3,
      Duration: 3.0,
      Setpoint: 50.0,
    }),
  ];

  const recipeVar = ns.addVariable({
    organizedBy: structuresFolder,
    browseName: "Recipe",
    nodeId: "s=Demo.Structures.Recipe",
    dataType: recipeType.nodeId,
    minimumSamplingInterval: 1000,
    value: {
      get: () =>
        new Variant({
          dataType: DataType.ExtensionObject,
          value: buildExtensionObject(addressSpace, recipeType, {
            Name: "DemoRecipe",
            Version: 3,
            Steps: recipeSteps,
          }),
        }),
    },
  });

  return {
    motorStatusType,
    motorStatusVar,
    sensorReadingType,
    sensorReadingVar,
    motorCommandType,
    recipeType,
    recipeStepType,
    recipeVar,
    currentMotor,
  };
}
