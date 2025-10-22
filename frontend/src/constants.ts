/**
 * Application-wide constants.
 *
 * Centralizes magic numbers and configuration values for maintainability.
 */

/**
 * WebSocket connection and reconnection settings.
 */
export const WEBSOCKET = {
  /** Interval between reconnection attempts in milliseconds */
  RECONNECT_INTERVAL: 1000,

  /** Maximum consecutive failures before triggering desynchronization */
  MAX_FAILURES: 5,

  /** Multiplier for failure reset interval (failures reset after RECONNECT_INTERVAL * this value) */
  FAILURE_RESET_MULTIPLIER: 15,
} as const;

/**
 * UI interaction timing and feedback durations.
 */
export const UI = {
  /** Duration to show "Copied!" feedback on copy button (milliseconds) */
  COPY_FEEDBACK_DURATION: 2000,

  /** Default toast notification duration (milliseconds) */
  TOAST_DURATION: 3000,

  /** Duration for informational toasts (milliseconds) */
  TOAST_INFO_DURATION: 3000,

  /** Success toast duration (milliseconds) */
  TOAST_SUCCESS_DURATION: 2000,
} as const;

/**
 * Document and editor limits.
 */
export const DOCUMENT = {
  /** Length of randomly generated document IDs */
  ID_LENGTH: 6,

  /** Characters allowed in document IDs */
  ID_CHARS: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
} as const;

/**
 * User profile settings.
 */
export const USER = {
  /** Maximum length for user display names */
  MAX_NAME_LENGTH: 25,

  /** System user ID (max uint64) used for system-generated operations and initial state */
  SYSTEM_USER_ID: 18446744073709551615,
} as const;
