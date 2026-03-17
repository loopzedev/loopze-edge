/**
 * TypeScript declaration for the Flint Function Node runtime.
 * These type definitions are injected into Monaco's JavaScript language service
 * so that the editor provides IntelliSense for the Flint API.
 */
export const flintTypeDefinitions = `
/** The incoming message object. */
interface FlintMessage {
  /** The message payload (any type). */
  payload: any;
  /** Message topic string. */
  topic: string;
  /** Unique message ID (read-only). */
  readonly _id: string;
  /** Creation timestamp (ISO 8601, read-only). */
  readonly _timestamp: string;
  /** Access any custom property. */
  [key: string]: any;
}

/** Node API for sending messages, logging, and setting status. */
interface FlintNodeAPI {
  /**
   * Send message(s) to output ports.
   * Pass a single message object to send on port 0,
   * or an array where each element corresponds to a port (null to skip a port).
   * @example node.send({ payload: "hello" })
   * @example node.send([{ payload: "port0" }, { payload: "port1" }])
   */
  send(msg: FlintMessage | Array<FlintMessage | null>): void;
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
}

/** Key-value context store. */
interface FlintContextStore {
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

/** The incoming message. */
declare var msg: FlintMessage;
/** Node API: send messages, log, set status. */
declare var node: FlintNodeAPI;
/** Global context store — shared across all flows. */
declare var global: FlintContextStore;
/** Flow-scoped context store — shared within the current flow. */
declare var flow: FlintContextStore;
`
