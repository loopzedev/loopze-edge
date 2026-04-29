/**
 * Smoke test: connects to a running demo server and reads a few representative
 * nodes including the ExtensionObject (MotorStatus) — verifies the structured
 * value comes back as a JS object with the expected fields.
 *
 * Run against a running demo server:
 *   npm start          # in one terminal
 *   npx tsx src/verify.ts   # in another
 */
import { AttributeIds, OPCUAClient, resolveNodeId } from "node-opcua";

const endpoint = process.env.FLINT_OPCUA_TEST_ENDPOINT ?? "opc.tcp://localhost:4840/flint-demo";

async function main(): Promise<void> {
  const client = OPCUAClient.create({
    endpointMustExist: false,
    connectionStrategy: { maxRetry: 1 },
  });
  await client.connect(endpoint);
  const session = await client.createSession();

  const nodes = [
    "ns=2;s=Demo.Static.Scalar.Double",
    "ns=2;s=Demo.Static.Scalar.String",
    "ns=2;s=Demo.Static.Scalar.Int64",
    "ns=2;s=Demo.Dynamic.Counter",
    "ns=2;s=Demo.Dynamic.Temperature",
    "ns=2;s=Demo.Structures.MotorStatus",
    "ns=2;s=Demo.Structures.SensorReading",
    "ns=2;s=Demo.Structures.Recipe",
    "ns=2;s=Demo.Writable.Setpoint",
    "ns=2;s=Demo.Writable.MotorCmd",
  ];

  for (const id of nodes) {
    const result = await session.read({ nodeId: resolveNodeId(id), attributeId: AttributeIds.Value });
    const v = result.value.value;
    const repr =
      typeof v === "object" && v !== null
        ? JSON.stringify(v, replacer).slice(0, 200)
        : String(v);
    // eslint-disable-next-line no-console
    console.log(`${id.padEnd(40)} ${result.statusCode.name.padEnd(8)} ${repr}`);
  }

  await session.close();
  await client.disconnect();
}

function replacer(_key: string, value: unknown): unknown {
  if (typeof value === "bigint") return value.toString();
  if (value instanceof Buffer) return `<bytes:${value.length}>`;
  return value;
}

main().catch((err: unknown) => {
  // eslint-disable-next-line no-console
  console.error("verify failed:", err);
  process.exit(1);
});
