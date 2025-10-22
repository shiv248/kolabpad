/**
 * Types related to Kolabpad WebSocket client and server communication
 */

/** A user currently editing the document */
export type UserInfo = {
  readonly name: string;
  readonly hue: number;
};

/** Cursor and selection data for a user */
export type CursorData = {
  cursors: number[];
  selections: [number, number][];
};

/** User operation from server history */
export type UserOperation = {
  id: number;
  operation: any;
};

/** Server message types */
export type ServerMsg = {
  Identity?: number;
  History?: {
    start: number;
    operations: UserOperation[];
  };
  Language?: {
    language: string;
    user_id: number;
    user_name: string;
  };
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
    user_id: number;
    user_name: string;
  };
};
