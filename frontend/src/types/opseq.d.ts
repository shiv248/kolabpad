/**
 * TypeScript definitions for the Go WASM OpSeq module.
 *
 * OpSeq is a JavaScript wrapper around the Go Operational Transformation library,
 * compiled to WebAssembly for browser-side OT operations.
 *
 * @see /cmd/ot-wasm-bridge/main.go - WASM bridge implementation
 * @see https://github.com/shiv248/operational-transformation-go - Go OT library source
 */

/**
 * Represents a pair of transformed operations returned by transform().
 */
export interface IOpSeqPair {
  /**
   * Returns the first transformed operation (a').
   */
  first(): IOpSeq;

  /**
   * Returns the second transformed operation (b').
   */
  second(): IOpSeq;
}

/**
 * An Operational Transformation operation sequence.
 *
 * Operations are composed of Insert, Delete, and Retain components that
 * describe changes to a text document.
 */
export interface IOpSeq {
  /**
   * Internal ID used by the WASM bridge to track Go objects.
   * @internal
   */
  readonly __opseq_id: number;

  /**
   * Delete n characters at the current position.
   * @param n - Number of characters to delete
   */
  delete(n: number): void;

  /**
   * Insert text at the current position.
   * @param text - Text to insert
   */
  insert(text: string): void;

  /**
   * Retain n characters at the current position.
   * @param n - Number of characters to retain
   */
  retain(n: number): void;

  /**
   * Compose this operation with another operation.
   *
   * Composition merges two sequential operations into a single operation
   * that has the same effect as applying both in sequence.
   *
   * @param other - The operation to compose with
   * @returns The composed operation, or null if composition fails
   */
  compose(other: IOpSeq): IOpSeq | null;

  /**
   * Transform this operation against another concurrent operation.
   *
   * Operational Transformation's core algorithm that resolves conflicts
   * between concurrent edits.
   *
   * @param other - The concurrent operation to transform against
   * @returns A pair of transformed operations (a', b'), or null if transform fails
   */
  transform(other: IOpSeq): IOpSeqPair | null;

  /**
   * Apply this operation to a document string.
   *
   * @param doc - The document string to apply the operation to
   * @returns The resulting document string, or null if application fails
   */
  apply(doc: string): string | null;

  /**
   * Invert this operation relative to a document.
   *
   * The inverse operation undoes the effect of this operation.
   *
   * @param doc - The document string the operation was applied to
   * @returns The inverse operation
   */
  invert(doc: string): IOpSeq;

  /**
   * Check if this operation is a no-op (does nothing).
   *
   * @returns true if the operation has no effect
   */
  is_noop(): boolean;

  /**
   * Get the base length of this operation.
   *
   * The base length is the length of the document before applying the operation.
   *
   * @returns The base document length
   */
  base_len(): number;

  /**
   * Get the target length of this operation.
   *
   * The target length is the length of the document after applying the operation.
   *
   * @returns The target document length
   */
  target_len(): number;

  /**
   * Transform a cursor/selection position through this operation.
   *
   * Used to update cursor positions when remote edits are applied.
   *
   * @param position - The cursor position to transform
   * @returns The transformed cursor position
   */
  transform_index(position: number): number;

  /**
   * Serialize this operation to a JSON string.
   *
   * @returns JSON representation of the operation
   */
  to_string(): string;
}

/**
 * OpSeq constructor namespace.
 *
 * Provides factory methods for creating OpSeq instances.
 */
export interface IOpSeqConstructor {
  /**
   * Create a new empty operation sequence.
   *
   * @returns A new OpSeq instance
   */
  new(): IOpSeq;

  /**
   * Deserialize an operation from JSON.
   *
   * @param json - JSON string representation of an operation
   * @returns Deserialized OpSeq instance, or null if parsing fails
   */
  from_str(json: string): IOpSeq | null;

  /**
   * Create a new operation sequence with pre-allocated capacity.
   *
   * @param capacity - Initial capacity for the operation list
   * @returns A new OpSeq instance with the specified capacity
   */
  with_capacity(capacity: number): IOpSeq;
}

/**
 * Global OpSeq constructor exported by the WASM module.
 *
 * Must be loaded before use by including the WASM module script.
 */
declare const OpSeq: IOpSeqConstructor;

export default OpSeq;
