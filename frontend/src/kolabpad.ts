import debounce from "lodash.debounce";
import type {
  IDisposable,
  IPosition,
  editor,
} from "monaco-editor/esm/vs/editor/editor.api";

import { WEBSOCKET } from "./constants";
import { logger } from "./logger";
import { zIndex } from "./theme";
import type { IOpSeq } from "./types/opseq";

// OpSeq is loaded from Go WASM (global variable set by cmd/ot-wasm)
// Type definition in ./types/opseq.d.ts
declare const OpSeq: {
  new(): IOpSeq;
  from_str(json: string): IOpSeq | null;
  with_capacity(capacity: number): IOpSeq;
} & {
  [key: string]: any; // Allow dynamic property access
};

/** Options passed in to the Kolabpad constructor. */
export type KolabpadOptions = {
  readonly uri: string;
  readonly editor: editor.IStandaloneCodeEditor;
  readonly onConnected?: () => void;
  readonly onDisconnected?: () => void;
  readonly onDesynchronized?: () => void;
  readonly onIdentity?: (userId: number) => void;
  readonly onChangeLanguage?: (language: string) => void;
  readonly onChangeUsers?: (users: Record<number, UserInfo>) => void;
  readonly onAuthError?: () => void;
  readonly onChangeOTP?: (otp: string | null, userId: number, userName: string) => void;
  readonly reconnectInterval?: number;
};

/** A user currently editing the document. */
export type UserInfo = {
  readonly name: string;
  readonly hue: number;
};

/** Browser client for Kolabpad. */
class Kolabpad {
  private ws?: WebSocket;
  private connecting?: boolean;
  private recentFailures: number = 0;
  private everConnected: boolean = false; // Track if we've ever successfully connected
  private disposed: boolean = false; // Track if instance has been disposed
  private readonly documentId: string; // Document ID extracted from URI
  private readonly model: editor.ITextModel;
  private readonly onChangeHandle: IDisposable;
  private readonly onCursorHandle: IDisposable;
  private readonly onSelectionHandle: IDisposable;
  private readonly beforeUnload: (event: BeforeUnloadEvent) => void;
  private readonly tryConnectId: number;
  private readonly resetFailuresId: number;

  // Client-server state
  private me: number = -1;
  private revision: number = 0;
  private outstanding?: IOpSeq;
  private buffer?: IOpSeq;
  private users: Record<number, UserInfo> = {};
  private userCursors: Record<number, CursorData> = {};
  private myInfo?: UserInfo;
  private cursorData: CursorData = { cursors: [], selections: [] };

  // Intermittent local editor state
  private lastValue: string = "";
  private ignoreChanges: boolean = false;
  private oldDecorations: string[] = [];

  // CSS style management (instance-scoped to prevent memory leaks)
  // eslint-disable-next-line no-undef
  private styleElement?: HTMLStyleElement;
  private styleSheet?: CSSStyleSheet;
  private generatedHues = new Set<number>();

  constructor(readonly options: KolabpadOptions) {
    this.model = options.editor.getModel()!;

    // Extract document ID from WebSocket URI for message validation
    const uriMatch = options.uri.match(/\/socket\/([^/?]+)/);
    this.documentId = uriMatch ? uriMatch[1] : "";
    logger.debug("[Kolabpad] Initialized for document:", this.documentId);

    this.onChangeHandle = options.editor.onDidChangeModelContent((e) =>
      this.onChange(e),
    );
    const cursorUpdate = debounce(() => this.sendCursorData(), 20);
    this.onCursorHandle = options.editor.onDidChangeCursorPosition((e) => {
      this.onCursor(e);
      cursorUpdate();
    });
    this.onSelectionHandle = options.editor.onDidChangeCursorSelection((e) => {
      this.onSelection(e);
      cursorUpdate();
    });
    this.beforeUnload = (event: BeforeUnloadEvent) => {
      if (this.outstanding) {
        event.preventDefault();
        event.returnValue = "";
      } else {
        delete event.returnValue;
      }
    };
    window.addEventListener("beforeunload", this.beforeUnload);

    const interval = options.reconnectInterval ?? WEBSOCKET.RECONNECT_INTERVAL;
    this.tryConnect();
    this.tryConnectId = window.setInterval(() => this.tryConnect(), interval);
    this.resetFailuresId = window.setInterval(
      () => (this.recentFailures = 0),
      WEBSOCKET.FAILURE_RESET_MULTIPLIER * interval,
    );
  }

  /** Destroy this Kolabpad instance and close any sockets. */
  dispose() {
    // Mark as disposed to ignore any stale WebSocket messages
    this.disposed = true;
    logger.debug("[Kolabpad] Disposed instance for document:", this.documentId);

    window.clearInterval(this.tryConnectId);
    window.clearInterval(this.resetFailuresId);
    this.onSelectionHandle.dispose();
    this.onCursorHandle.dispose();
    this.onChangeHandle.dispose();
    window.removeEventListener("beforeunload", this.beforeUnload);
    this.ws?.close();

    // Clean up CSS styles to prevent memory leak
    if (this.styleElement) {
      this.styleElement.remove();
      this.styleElement = undefined;
      this.styleSheet = undefined;
      this.generatedHues.clear();
    }
  }

  /** Try to set the language of the editor, if connected. */
  setLanguage(language: string): boolean {
    this.ws?.send(`{"SetLanguage":${JSON.stringify(language)}}`);
    return this.ws !== undefined;
  }

  /** Set the user's information. */
  setInfo(info: UserInfo) {
    this.myInfo = info;
    this.sendInfo();
  }

  /** Get the current user's ID assigned by the server. Returns -1 if not connected yet. */
  getUserId(): number {
    return this.me;
  }

  /**
   * Attempts a WebSocket connection.
   *
   * Safety Invariant: Until this WebSocket connection is closed, no other
   * connections will be attempted because either `this.ws` or
   * `this.connecting` will be set to a truthy value.
   *
   * Liveness Invariant: After this WebSocket connection closes, either through
   * error or successful end, both `this.connecting` and `this.ws` will be set
   * to falsy values.
   */
  private tryConnect() {
    if (this.connecting || this.ws) return;
    this.connecting = true;
    const ws = new WebSocket(this.options.uri);
    ws.onopen = () => {
      this.connecting = false;
      this.ws = ws;
      this.everConnected = true; // Mark that we've successfully connected at least once
      this.options.onConnected?.();
      this.users = {};
      this.options.onChangeUsers?.(this.users);
      this.sendInfo();
      this.sendCursorData();
      if (this.outstanding) {
        this.sendOperation(this.outstanding);
      }
    };
    ws.onclose = (event) => {
      if (this.ws) {
        this.ws = undefined;
        this.options.onDisconnected?.();
        if (++this.recentFailures >= WEBSOCKET.MAX_FAILURES) {
          // If we disconnect MAX_FAILURES times within FAILURE_RESET_MULTIPLIER reconnection intervals,
          // then the client is likely desynchronized and needs to refresh.
          this.dispose();
          this.options.onDesynchronized?.();
        }
      } else {
        this.connecting = false;
        // Check if this was an authentication error (connection refused during handshake)
        // WebSocket close code 1002 = protocol error, or 1006 = abnormal closure
        // Only treat as auth error if we've never successfully connected before
        if ((event.code === 1006 || event.code === 1002) && !this.everConnected) {
          // Never successfully connected - likely auth error
          this.options.onAuthError?.();
        }
      }
    };
    ws.onmessage = ({ data }) => {
      if (typeof data === "string") {
        this.handleMessage(JSON.parse(data));
      }
    };
  }

  private handleMessage(msg: ServerMsg) {
    // Ignore messages if this instance has been disposed (prevents stale messages)
    if (this.disposed) {
      logger.warn("[WebSocket] Ignoring message - instance disposed for document:", this.documentId);
      return;
    }

    if (msg.Identity !== undefined) {
      this.me = msg.Identity;
      logger.debug("[Identity] Assigned ID:", this.me);
      this.options.onIdentity?.(this.me);
    } else if (msg.History !== undefined) {
      const { start, operations } = msg.History;
      logger.debug(`[History] Received ${operations.length} operations from revision ${start}`);
      if (start > this.revision) {
        logger.warn("History message has start greater than last operation.");
        this.ws?.close();
        return;
      }
      for (let i = this.revision - start; i < operations.length; i++) {
        let { id, operation } = operations[i];
        const rawOp = operation;
        this.revision++;
        if (id === this.me) {
          logger.debug(`[History] Rev ${this.revision}: Our operation acknowledged (user=${id})`);
          this.serverAck();
        } else {
          operation = OpSeq.from_str(JSON.stringify(operation));
          logger.debug(`[History] Rev ${this.revision}: Remote operation from user ${id}:`, this.formatOperation(rawOp));
          this.applyServer(operation);
        }
      }
    } else if (msg.Language !== undefined) {
      logger.debug(`[Language] Changed to: ${msg.Language}`);
      this.options.onChangeLanguage?.(msg.Language);
    } else if (msg.UserInfo !== undefined) {
      const { id, info } = msg.UserInfo;
      if (id !== this.me) {
        this.users = { ...this.users };
        if (info) {
          logger.debug(`[UserInfo] User ${id} joined: ${info.name} (hue=${info.hue})`);
          this.users[id] = info;
        } else {
          logger.debug(`[UserInfo] User ${id} left`);
          delete this.users[id];
          delete this.userCursors[id];
        }
        this.updateCursors();
        this.options.onChangeUsers?.(this.users);
      }
    } else if (msg.UserCursor !== undefined) {
      const { id, data } = msg.UserCursor;
      if (id !== this.me) {
        logger.debug(`[UserCursor] User ${id}: cursors=${data.cursors}, selections=${JSON.stringify(data.selections)}`);
        this.userCursors[id] = data;
        this.updateCursors();
      }
    } else if (msg.OTP !== undefined) {
      const { otp, user_id, user_name } = msg.OTP;
      logger.debug(`[OTP] Changed to: ${otp || 'disabled'} by user ${user_id} (${user_name})`);
      this.options.onChangeOTP?.(otp, user_id, user_name);
    }
  }

  private serverAck() {
    if (!this.outstanding) {
      logger.warn("Received serverAck with no outstanding operation.");
      return;
    }
    logger.debug(`[ServerAck] Outstanding cleared, buffer=${this.buffer ? 'pending' : 'none'}`);
    this.outstanding = this.buffer;
    this.buffer = undefined;
    if (this.outstanding) {
      logger.debug(`[ServerAck] Sending buffered operation:`, this.formatOperation(JSON.parse(this.outstanding.to_string())));
      this.sendOperation(this.outstanding);
    }
  }

  private applyServer(operation: IOpSeq) {
    const fullDoc = this.model.getValue();
    const beforeDoc = fullDoc.slice(0, 50);
    const beforeTruncated = fullDoc.length > 50;

    if (this.outstanding) {
      logger.debug(`[ApplyServer] Transforming against outstanding operation`);
      const pair = this.outstanding.transform(operation);
      if (!pair) {
        logger.error("[ApplyServer] Transform failed against outstanding - desynchronized");
        this.dispose();
        this.options.onDesynchronized?.();
        return;
      }
      this.outstanding = pair.first();
      operation = pair.second();

      if (this.buffer) {
        logger.debug(`[ApplyServer] Transforming against buffered operation`);
        const bufferPair = this.buffer.transform(operation);
        if (!bufferPair) {
          logger.error("[ApplyServer] Transform failed against buffer - desynchronized");
          this.dispose();
          this.options.onDesynchronized?.();
          return;
        }
        this.buffer = bufferPair.first();
        operation = bufferPair.second();
      }
    }
    logger.debug(`[ApplyServer] Applying to document (before): "${beforeDoc}${beforeTruncated ? '...' : ''}"`);
    this.applyOperation(operation);
    const fullDocAfter = this.model.getValue();
    const afterDoc = fullDocAfter.slice(0, 50);
    const afterTruncated = fullDocAfter.length > 50;
    logger.debug(`[ApplyServer] Applied (after): "${afterDoc}${afterTruncated ? '...' : ''}"`);
  }

  private applyClient(operation: IOpSeq) {
    const opDetails = this.formatOperation(JSON.parse(operation.to_string()));
    if (!this.outstanding) {
      logger.debug(`[ApplyClient] Sending operation (no outstanding):`, opDetails);
      this.sendOperation(operation);
      this.outstanding = operation;
    } else if (!this.buffer) {
      logger.debug(`[ApplyClient] Buffering operation (outstanding exists):`, opDetails);
      this.buffer = operation;
    } else {
      logger.debug(`[ApplyClient] Composing with buffer:`, opDetails);
      const composed = this.buffer.compose(operation);
      if (composed) {
        this.buffer = composed;
      }
    }
    this.transformCursors(operation);
  }

  private sendOperation(operation: IOpSeq) {
    const op = operation.to_string();
    logger.debug(`[SendOperation] Sending at revision ${this.revision}:`, this.formatOperation(JSON.parse(op)));
    this.ws?.send(`{"Edit":{"revision":${this.revision},"operation":${op}}}`);
  }

  private sendInfo() {
    if (this.myInfo) {
      this.ws?.send(`{"ClientInfo":${JSON.stringify(this.myInfo)}}`);
    }
  }

  private sendCursorData() {
    if (!this.buffer) {
      this.ws?.send(`{"CursorData":${JSON.stringify(this.cursorData)}}`);
    }
  }

  private applyOperation(operation: IOpSeq) {
    if (operation.is_noop()) return;

    this.ignoreChanges = true;
    const ops: (string | number)[] = JSON.parse(operation.to_string());
    let index = 0;

    for (const op of ops) {
      if (typeof op === "string") {
        // Insert
        const pos = unicodePosition(this.model, index);
        index += unicodeLength(op);

        this.model.pushEditOperations(
          this.options.editor.getSelections(),
          [
            {
              range: {
                startLineNumber: pos.lineNumber,
                startColumn: pos.column,
                endLineNumber: pos.lineNumber,
                endColumn: pos.column,
              },
              text: op,
              forceMoveMarkers: true,
            },
          ],
          () => null,
        );
      } else if (op >= 0) {
        // Retain
        index += op;
      } else {
        // Delete
        const chars = -op;
        var from = unicodePosition(this.model, index);
        var to = unicodePosition(this.model, index + chars);

        this.model.pushEditOperations(
          this.options.editor.getSelections(),
          [
            {
              range: {
                startLineNumber: from.lineNumber,
                startColumn: from.column,
                endLineNumber: to.lineNumber,
                endColumn: to.column,
              },
              text: "",
              forceMoveMarkers: true,
            },
          ],
          () => null,
        );
      }
    }

    this.lastValue = this.model.getValue();
    this.ignoreChanges = false;

    this.transformCursors(operation);
  }

  private transformCursors(operation: IOpSeq) {
    for (const data of Object.values(this.userCursors)) {
      data.cursors = data.cursors.map((c) => operation.transform_index(c));
      data.selections = data.selections.map(([s, e]) => [
        operation.transform_index(s),
        operation.transform_index(e),
      ]);
    }
    this.updateCursors();
  }

  private updateCursors() {
    const decorations: editor.IModelDeltaDecoration[] = [];

    for (const [id, data] of Object.entries(this.userCursors)) {
      if (id in this.users) {
        const { hue, name } = this.users[id as any];
        this.generateCssStyles(hue);

        for (const cursor of data.cursors) {
          const position = unicodePosition(this.model, cursor);
          decorations.push({
            options: {
              className: `remote-cursor-${hue}`,
              stickiness: 1,
              zIndex: zIndex.editorCursor,
            },
            range: {
              startLineNumber: position.lineNumber,
              startColumn: position.column,
              endLineNumber: position.lineNumber,
              endColumn: position.column,
            },
          });
        }
        for (const selection of data.selections) {
          const position = unicodePosition(this.model, selection[0]);
          const positionEnd = unicodePosition(this.model, selection[1]);
          decorations.push({
            options: {
              className: `remote-selection-${hue}`,
              hoverMessage: {
                value: name,
              },
              stickiness: 1,
              zIndex: zIndex.editorSelection,
            },
            range: {
              startLineNumber: position.lineNumber,
              startColumn: position.column,
              endLineNumber: positionEnd.lineNumber,
              endColumn: positionEnd.column,
            },
          });
        }
      }
    }

    this.oldDecorations = this.model.deltaDecorations(
      this.oldDecorations,
      decorations,
    );
  }

  private onChange(event: editor.IModelContentChangedEvent) {
    if (!this.ignoreChanges) {
      const content = this.lastValue;
      const contentLength = unicodeLength(content);
      let offset = 0;

      let operation = OpSeq.new();
      operation.retain(contentLength);
      event.changes.sort((a, b) => b.rangeOffset - a.rangeOffset);
      for (const change of event.changes) {
        // The following dance is necessary to convert from UTF-16 indices (evil
        // encoding-dependent JavaScript representation) to portable Unicode
        // codepoint indices.
        const { text, rangeOffset, rangeLength } = change;
        const initialLength = unicodeLength(content.slice(0, rangeOffset));
        const deletedLength = unicodeLength(
          content.slice(rangeOffset, rangeOffset + rangeLength),
        );
        const restLength =
          contentLength + offset - initialLength - deletedLength;
        const changeOp = OpSeq.new();
        changeOp.retain(initialLength);
        changeOp.delete(deletedLength);
        changeOp.insert(text);
        changeOp.retain(restLength);
        const composed = operation.compose(changeOp);
        if (!composed) {
          logger.error("[onChange] Compose failed - desynchronized");
          this.dispose();
          this.options.onDesynchronized?.();
          return;
        }
        operation = composed;
        offset += changeOp.target_len() - changeOp.base_len();
      }
      this.applyClient(operation);
      this.lastValue = this.model.getValue();
    }
  }

  private onCursor(event: editor.ICursorPositionChangedEvent) {
    const cursors = [event.position, ...event.secondaryPositions];
    this.cursorData.cursors = cursors.map((p) => unicodeOffset(this.model, p));
  }

  private onSelection(event: editor.ICursorSelectionChangedEvent) {
    const selections = [event.selection, ...event.secondarySelections];
    this.cursorData.selections = selections.map((s) => [
      unicodeOffset(this.model, s.getStartPosition()),
      unicodeOffset(this.model, s.getEndPosition()),
    ]);
  }

  /** Format an operation for readable debug output */
  private formatOperation(ops: (string | number)[]): string {
    const parts: string[] = [];
    for (const op of ops) {
      if (typeof op === "string") {
        // Insert
        const preview = op.length > 20 ? op.slice(0, 20) + "..." : op;
        parts.push(`Insert("${preview}", ${op.length} chars)`);
      } else if (op >= 0) {
        // Retain
        parts.push(`Retain(${op})`);
      } else {
        // Delete
        parts.push(`Delete(${-op})`);
      }
    }
    return parts.join(", ");
  }

  /**
   * Add CSS styles for a remote user's cursor and selection.
   * Instance method to prevent memory leaks across document switches.
   */
  private generateCssStyles(hue: number) {
    if (this.generatedHues.has(hue)) return;

    // Create stylesheet on first use for this document
    if (!this.styleSheet) {
      this.styleElement = document.createElement("style");
      document.head.appendChild(this.styleElement);
      this.styleSheet = this.styleElement.sheet as CSSStyleSheet;
    }

    this.generatedHues.add(hue);

    // Add rules to instance-scoped stylesheet
    this.styleSheet.insertRule(
      `.monaco-editor .remote-selection-${hue} { background-color: hsla(${hue}, 90%, 80%, 0.5); }`,
      this.styleSheet.cssRules.length
    );
    this.styleSheet.insertRule(
      `.monaco-editor .remote-cursor-${hue} { border-left: 2px solid hsl(${hue}, 90%, 25%); }`,
      this.styleSheet.cssRules.length
    );
  }
}

type UserOperation = {
  id: number;
  operation: any;
};

type CursorData = {
  cursors: number[];
  selections: [number, number][];
};

type ServerMsg = {
  Identity?: number;
  History?: {
    start: number;
    operations: UserOperation[];
  };
  Language?: string;
  UserInfo?: {
    id: number;
    info: UserInfo | null;
  };
  UserCursor?: {
    id: number;
    data: CursorData;
  };
  OTP?: {
    otp: string | null;
  };
};

/** Returns the number of Unicode codepoints in a string. */
function unicodeLength(str: string): number {
  let length = 0;
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  for (const c of str) ++length;
  return length;
}

/** Returns the number of Unicode codepoints before a position in the model. */
function unicodeOffset(model: editor.ITextModel, pos: IPosition): number {
  const value = model.getValue();
  const offsetUTF16 = model.getOffsetAt(pos);
  return unicodeLength(value.slice(0, offsetUTF16));
}

/** Returns the position after a certain number of Unicode codepoints. */
function unicodePosition(model: editor.ITextModel, offset: number): IPosition {
  const value = model.getValue();
  let offsetUTF16 = 0;
  for (const c of value) {
    // Iterate over Unicode codepoints
    if (offset <= 0) break;
    offsetUTF16 += c.length;
    offset -= 1;
  }
  return model.getPositionAt(offsetUTF16);
}

export default Kolabpad;
