import {
  DataType,
  type Namespace,
  type UAObject,
  type UAVariable,
  Variant,
  VariantArrayType,
} from "node-opcua";

export interface DynamicScalars {
  counter: UAVariable;
  sine: UAVariable;
  sawtooth: UAVariable;
  random: UAVariable;
  temperature: UAVariable;
  pressure: UAVariable;
}

const SCALAR_TYPES: ReadonlyArray<{ name: string; dt: DataType; sample: unknown }> = [
  { name: "Boolean", dt: DataType.Boolean, sample: true },
  { name: "SByte", dt: DataType.SByte, sample: -7 },
  { name: "Byte", dt: DataType.Byte, sample: 42 },
  { name: "Int16", dt: DataType.Int16, sample: -1234 },
  { name: "UInt16", dt: DataType.UInt16, sample: 1234 },
  { name: "Int32", dt: DataType.Int32, sample: -100000 },
  { name: "UInt32", dt: DataType.UInt32, sample: 100000 },
  // node-opcua represents 64-bit integers as [high32, low32] tuples
  { name: "Int64", dt: DataType.Int64, sample: [0xffffffff, 0xffffffd6] }, // -42
  { name: "UInt64", dt: DataType.UInt64, sample: [0, 42] },
  { name: "Float", dt: DataType.Float, sample: 3.1415927 },
  { name: "Double", dt: DataType.Double, sample: 2.718281828459045 },
  { name: "String", dt: DataType.String, sample: "hello flint" },
  { name: "DateTime", dt: DataType.DateTime, sample: new Date("2026-01-01T00:00:00Z") },
  { name: "ByteString", dt: DataType.ByteString, sample: Buffer.from([0xde, 0xad, 0xbe, 0xef]) },
];

export function addScalars(
  ns: Namespace,
  staticFolder: UAObject,
  dynamicFolder: UAObject,
): DynamicScalars {
  const scalarFolder = ns.addFolder(staticFolder, { browseName: "Scalar" });
  for (const { name, dt, sample } of SCALAR_TYPES) {
    ns.addVariable({
      organizedBy: scalarFolder,
      browseName: name,
      nodeId: `s=Demo.Static.Scalar.${name}`,
      dataType: dt,
      // Wrap in a Variant so we can pin arrayType=Scalar — needed for Int64/UInt64
      // where node-opcua otherwise can't tell a [high,low] tuple from an array.
      value: new Variant({
        dataType: dt,
        arrayType: VariantArrayType.Scalar,
        value: sample,
      }),
      minimumSamplingInterval: 0,
    });
  }

  // GUID and LocalizedText extras
  ns.addVariable({
    organizedBy: scalarFolder,
    browseName: "Guid",
    nodeId: "s=Demo.Static.Scalar.Guid",
    dataType: DataType.Guid,
    value: { dataType: DataType.Guid, value: "00112233-4455-6677-8899-aabbccddeeff" },
  });
  ns.addVariable({
    organizedBy: scalarFolder,
    browseName: "LocalizedText",
    nodeId: "s=Demo.Static.Scalar.LocalizedText",
    dataType: DataType.LocalizedText,
    value: {
      dataType: DataType.LocalizedText,
      value: { locale: "en", text: "Localized demo text" },
    },
  });

  // Dynamic values – held in closures, server pushes updates via simulation tick.
  let counterVal = 0;
  const counter = ns.addVariable({
    organizedBy: dynamicFolder,
    browseName: "Counter",
    nodeId: "s=Demo.Dynamic.Counter",
    dataType: DataType.Int32,
    minimumSamplingInterval: 100,
    value: {
      get: () => new Variant({ dataType: DataType.Int32, value: counterVal }),
    },
  });
  Object.defineProperty(counter, "_setVal", {
    value: (v: number) => {
      counterVal = v;
    },
    enumerable: false,
  });

  const sine = makeDynamicDouble(ns, dynamicFolder, "Sine", "s=Demo.Dynamic.Sine", 0);
  const sawtooth = makeDynamicDouble(
    ns,
    dynamicFolder,
    "Sawtooth",
    "s=Demo.Dynamic.Sawtooth",
    0,
  );
  const random = makeDynamicDouble(ns, dynamicFolder, "Random", "s=Demo.Dynamic.Random", 0);
  const temperature = makeDynamicDouble(
    ns,
    dynamicFolder,
    "Temperature",
    "s=Demo.Dynamic.Temperature",
    20.0,
  );
  const pressure = makeDynamicDouble(
    ns,
    dynamicFolder,
    "Pressure",
    "s=Demo.Dynamic.Pressure",
    1.013,
  );

  return { counter, sine, sawtooth, random, temperature, pressure };
}

function makeDynamicDouble(
  ns: Namespace,
  folder: UAObject,
  name: string,
  nodeIdPath: string,
  initial: number,
): UAVariable {
  let v = initial;
  const variable = ns.addVariable({
    organizedBy: folder,
    browseName: name,
    nodeId: nodeIdPath,
    dataType: DataType.Double,
    minimumSamplingInterval: 100,
    value: {
      get: () => new Variant({ dataType: DataType.Double, value: v }),
    },
  });
  Object.defineProperty(variable, "_setVal", {
    value: (next: number) => {
      v = next;
    },
    enumerable: false,
  });
  return variable;
}

// Helper used by simulation to push values into dynamic variables.
export function setDynamic(variable: UAVariable, value: number): void {
  const setter = (variable as unknown as { _setVal?: (v: number) => void })._setVal;
  if (setter) setter(value);
}
