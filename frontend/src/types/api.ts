/**
 * API request and response types
 */

/** Request to protect a document with OTP */
export interface ProtectDocumentRequest {
  otp: string;
}

/** Request to unprotect a document */
export interface UnprotectDocumentRequest {
  otp: string;
}

/** Generic API error response */
export interface ApiErrorResponse {
  error: string;
}
