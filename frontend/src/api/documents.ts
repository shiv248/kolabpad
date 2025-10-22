/**
 * Document-related API endpoints
 */

import { apiFetch } from './client';
import type { ProtectDocumentRequest, UnprotectDocumentRequest } from '../types/api';

/**
 * Enable OTP protection for a document
 * @param documentId - The document ID
 * @param userId - ID of the user enabling protection
 * @param userName - Name of the user enabling protection
 * @returns Server response with OTP token
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
 * Disable OTP protection for a document
 * @param documentId - The document ID
 * @param userId - ID of the user disabling protection
 * @param userName - Name of the user disabling protection
 * @param otp - Current OTP token (required for security)
 * @returns Void on success
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
