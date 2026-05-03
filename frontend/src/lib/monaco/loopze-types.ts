/**
 * TypeScript declaration for the LOOPZE Function Node runtime.
 * These type definitions are injected into Monaco's JavaScript language service
 * so that the editor provides IntelliSense for the LOOPZE API.
 */
export const loopzeTypeDefinitions = `
/** The incoming message object. */
interface LoopzeMessage {
  /** The message payload (any type). */
  payload?: any;
  /** Message topic string. */
  topic?: string;
  /** Unique message ID (read-only). */
  readonly _id?: string;
  /** Creation timestamp (ISO 8601, read-only). */
  readonly _timestamp?: string;
  /** Access any custom property. */
  [key: string]: any;
}

/** Node API for sending messages, logging, and setting status. */
interface LoopzeNodeAPI {
  /**
   * Send message(s) to output ports.
   * Pass a single message object to send on port 0,
   * or an array where each element corresponds to a port (null to skip a port).
   * @example node.send({ payload: "hello" })
   * @example node.send([{ payload: "port0" }, { payload: "port1" }])
   */
  send(msg: LoopzeMessage | Array<LoopzeMessage | null>): void;
  /** Log a debug message. */
  log(value: any): void;
  /** Log a warning message. */
  warn(value: any): void;
  /** Log an error message. */
  error(value: any): void;
  /**
   * Set the node status indicator.
   * @param fill The status color: 'green' | 'red' | 'yellow' | 'blue' | 'grey'
   * @param text The status text displayed below the node.
   */
  status(fill: 'green' | 'red' | 'yellow' | 'blue' | 'grey', text: string): void;
  /**
   * Read a value from the node-scoped in-memory context.
   * This context is private to this node instance and survives across messages,
   * but is lost on redeploy. Use global/flow context for persistence.
   * @param key The key to look up.
   * @returns The stored value, or undefined if the key does not exist.
   */
  get(key: string): any;
  /**
   * Write a value to the node-scoped in-memory context.
   * @param key The key to store under.
   * @param value The value to store.
   */
  set(key: string, value: any): void;
  /**
   * Delete a key from the node-scoped in-memory context.
   * @param key The key to delete.
   */
  delete(key: string): void;
}

/** Key-value context store. */
interface LoopzeContextStore {
  /**
   * Read a value from the context store.
   * @param key The key to look up.
   * @param persistent If true, reads from the persistent (file-backed) store instead of memory.
   */
  get(key: string, persistent?: boolean): any;
  /**
   * Write a value to the context store.
   * @param key The key to store under.
   * @param value The value to store (must be JSON-serializable).
   * @param persistent If true, writes to the persistent (file-backed) store instead of memory.
   */
  set(key: string, value: any, persistent?: boolean): void;
  /**
   * Delete a key from the context store.
   * @param key The key to delete.
   * @param persistent If true, deletes from the persistent store.
   */
  delete(key: string, persistent?: boolean): void;
  /**
   * List all keys in the context store.
   * @param persistent If true, lists keys from the persistent store.
   */
  keys(persistent?: boolean): string[];
}

/** A byte buffer for binary data manipulation (Node.js Buffer compatible). */
interface LoopzeBuffer {
  /** Number of bytes in the buffer. */
  readonly length: number;

  // ── Read — Integer ──
  readUInt8(offset?: number): number;
  readInt8(offset?: number): number;
  readUInt16BE(offset?: number): number;
  readUInt16LE(offset?: number): number;
  readInt16BE(offset?: number): number;
  readInt16LE(offset?: number): number;
  readUInt32BE(offset?: number): number;
  readUInt32LE(offset?: number): number;
  readInt32BE(offset?: number): number;
  readInt32LE(offset?: number): number;
  readBigUInt64BE(offset?: number): number;
  readBigUInt64LE(offset?: number): number;
  readBigInt64BE(offset?: number): number;
  readBigInt64LE(offset?: number): number;
  readUIntBE(offset: number, byteLength: number): number;
  readUIntLE(offset: number, byteLength: number): number;
  readIntBE(offset: number, byteLength: number): number;
  readIntLE(offset: number, byteLength: number): number;

  // ── Read — Float ──
  readFloatBE(offset?: number): number;
  readFloatLE(offset?: number): number;
  readDoubleBE(offset?: number): number;
  readDoubleLE(offset?: number): number;

  // ── Write — Integer ──
  writeUInt8(value: number, offset?: number): void;
  writeInt8(value: number, offset?: number): void;
  writeUInt16BE(value: number, offset?: number): void;
  writeUInt16LE(value: number, offset?: number): void;
  writeInt16BE(value: number, offset?: number): void;
  writeInt16LE(value: number, offset?: number): void;
  writeUInt32BE(value: number, offset?: number): void;
  writeUInt32LE(value: number, offset?: number): void;
  writeInt32BE(value: number, offset?: number): void;
  writeInt32LE(value: number, offset?: number): void;
  writeBigUInt64BE(value: number, offset?: number): void;
  writeBigUInt64LE(value: number, offset?: number): void;
  writeBigInt64BE(value: number, offset?: number): void;
  writeBigInt64LE(value: number, offset?: number): void;
  writeUIntBE(value: number, offset: number, byteLength: number): void;
  writeUIntLE(value: number, offset: number, byteLength: number): void;
  writeIntBE(value: number, offset: number, byteLength: number): void;
  writeIntLE(value: number, offset: number, byteLength: number): void;

  // ── Write — Float ──
  writeFloatBE(value: number, offset?: number): void;
  writeFloatLE(value: number, offset?: number): void;
  writeDoubleBE(value: number, offset?: number): void;
  writeDoubleLE(value: number, offset?: number): void;

  // ── Swap ──
  /** Swap byte order in 16-bit pairs. */
  swap16(): LoopzeBuffer;
  /** Swap byte order in 32-bit groups. */
  swap32(): LoopzeBuffer;
  /** Swap byte order in 64-bit groups. */
  swap64(): LoopzeBuffer;

  // ── Conversion ──
  /**
   * Convert buffer to string.
   * @param encoding 'hex' | 'base64' | undefined (UTF-8)
   */
  toString(encoding?: 'hex' | 'base64'): string;
  /** Return buffer as byte-value array, e.g. [72, 101, 108]. */
  toJSON(): number[];
  /** Return a copy of bytes from start to end. */
  slice(start: number, end?: number): LoopzeBuffer;
  /** Copy bytes into target buffer. Returns number of bytes copied. */
  copy(target: LoopzeBuffer, targetStart?: number, sourceStart?: number, sourceEnd?: number): number;
}

/** Static methods for creating Buffer instances. */
interface LoopzeBufferConstructor {
  /**
   * Create a zero-filled buffer of the given size.
   * @example Buffer.alloc(8)
   */
  alloc(size: number): LoopzeBuffer;
  /**
   * Create a buffer from data.
   * @example Buffer.from([0x48, 0x65, 0x6C])
   * @example Buffer.from("Hello")
   * @example Buffer.from("48656c6c6f", "hex")
   * @example Buffer.from("SGVsbG8=", "base64")
   */
  from(data: number[] | string, encoding?: 'hex' | 'base64'): LoopzeBuffer;
  /** Concatenate multiple buffers into one. */
  concat(list: LoopzeBuffer[]): LoopzeBuffer;
}

/** The incoming message. */
declare var msg: LoopzeMessage;
/** Node API: send messages, log, set status. */
declare var node: LoopzeNodeAPI;
/** Global context store — shared across all flows. */
declare var global: LoopzeContextStore;
/** Flow-scoped context store — shared within the current flow. */
declare var flow: LoopzeContextStore;
/** Buffer API for byte manipulation (Node.js compatible). */
declare var Buffer: LoopzeBufferConstructor;
`
