/**
 * Document-related API endpoints
 */

import { apiFetch } from './client';
import type { ProtectDocumentRequest, UnprotectDocumentRequest } from '../types/api';

/**
 * Enables OTP (One-Time Password) protection for a document.
 *
 * When protection is enabled, the document requires a valid OTP token in the URL
 * to access. The server generates a unique token and broadcasts the change to all
 * connected clients.
 *
 * @param documentId - The document ID to protect
 * @param userId - ID of the user enabling protection
 * @param userName - Name of the user (for audit trail)
 *
 * @returns Promise resolving to object containing the generated OTP token
 *
 * @throws {ApiError} When the API request fails
 *
 * @example
 * ```ts
 * const { otp } = await protectDocument('abc123', 42, 'Alice');
 * console.log('Document protected with OTP:', otp);
 * ```
 */
export async function protectDocument(
  documentId: string,
  userId: number,
  userName: string
): Promise<{ otp: string }> {
  return apiFetch(`/api/document/${documentId}/protect`, {
    method: 'POST',
    body: {
      user_id: userId,
      user_name: userName,
    } as ProtectDocumentRequest,
  });
}

/**
 * Disables OTP protection for a document.
 *
 * Requires the current OTP token for security - only users with the valid OTP
 * can disable protection. The server broadcasts the change to all connected clients.
 *
 * @param documentId - The document ID to unprotect
 * @param userId - ID of the user disabling protection
 * @param userName - Name of the user (for audit trail)
 * @param otp - Current OTP token (required for authorization)
 *
 * @returns Promise resolving when protection is disabled
 *
 * @throws {ApiError} When the API request fails (e.g., invalid OTP)
 *
 * @example
 * ```ts
 * await unprotectDocument('abc123', 42, 'Alice', 'current-otp-token');
 * console.log('Document protection removed');
 * ```
 */
export async function unprotectDocument(
  documentId: string,
  userId: number,
  userName: string,
  otp: string
): Promise<void> {
  return apiFetch(`/api/document/${documentId}/protect`, {
    method: 'DELETE',
    body: {
      user_id: userId,
      user_name: userName,
      otp,
    } as UnprotectDocumentRequest,
  });
}
