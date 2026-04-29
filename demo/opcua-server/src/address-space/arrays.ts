import {
  DataType,
  type Namespace,
  type UAObject,
  Variant,
  VariantArrayType,
} from "node-opcua";

export function addArrays(ns: Namespace, staticFolder: UAObject): void {
  const arrayFolder = ns.addFolder(staticFolder, { browseName: "Array" });

  ns.addVariable({
    organizedBy: arrayFolder,
    browseName: "Int32Array",
    nodeId: "s=Demo.Static.Array.Int32Array",
    dataType: DataType.Int32,
    valueRank: 1,
    arrayDimensions: [10],
    value: new Variant({
      dataType: DataType.Int32,
      arrayType: VariantArrayType.Array,
      value: new Int32Array([0, 1, 2, 3, 4, 5, 6, 7, 8, 9]),
    }),
  });

  ns.addVariable({
    organizedBy: arrayFolder,
    browseName: "DoubleArray",
    nodeId: "s=Demo.Static.Array.DoubleArray",
    dataType: DataType.Double,
    valueRank: 1,
    arrayDimensions: [5],
    value: new Variant({
      dataType: DataType.Double,
      arrayType: VariantArrayType.Array,
      value: new Float64Array([1.1, 2.2, 3.3, 4.4, 5.5]),
    }),
  });

  ns.addVariable({
    organizedBy: arrayFolder,
    browseName: "StringArray",
    nodeId: "s=Demo.Static.Array.StringArray",
    dataType: DataType.String,
    valueRank: 1,
    arrayDimensions: [3],
    value: new Variant({
      dataType: DataType.String,
      arrayType: VariantArrayType.Array,
      value: ["alpha", "beta", "gamma"],
    }),
  });

  ns.addVariable({
    organizedBy: arrayFolder,
    browseName: "ByteArrayAsByteString",
    nodeId: "s=Demo.Static.Array.ByteArrayAsByteString",
    dataType: DataType.ByteString,
    value: new Variant({
      dataType: DataType.ByteString,
      value: Buffer.from([0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08]),
    }),
  });
}
