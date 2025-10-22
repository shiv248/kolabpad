/**
 * Custom hook for managing OTP (One-Time Password) protection state
 * Handles local state, URL synchronization, server broadcasts, and API calls
 */

import { useState, useEffect } from 'react';
import { useToast } from '@chakra-ui/react';
import { protectDocument, unprotectDocument } from '../api/documents';
import { getOtpFromUrl } from '../utils/url';
import { logger } from '../logger';
import { UI } from '../constants';
import type { OTPBroadcast } from '../types';

/**
 * Options for useOTPSync hook
 */
export interface UseOTPSyncOptions {
  /** The document ID being protected */
  documentId: string;
  /** ID of the current user */
  currentUserId: number;
  /** Name of the current user */
  currentUserName: string;
  /** Server broadcast containing OTP state updates */
  otpBroadcast: OTPBroadcast | undefined;
}

/**
 * Return value from useOTPSync hook
 */
export interface UseOTPSyncResult {
  /** Current OTP token or null if disabled */
  otp: string | null;
  /** Whether OTP protection is currently enabled */
  otpEnabled: boolean;
  /** Whether an API request is in progress */
  isToggling: boolean;
  /** Shareable document URL (includes OTP if enabled) */
  documentUrl: string;
  /** Function to enable or disable OTP protection */
  toggleOTP: (enabled: boolean) => Promise<void>;
}

/**
 * Manages OTP (One-Time Password) protection state with server synchronization.
 *
 * This hook handles:
 * - Initialization from URL parameters
 * - Server broadcast synchronization (all clients stay in sync)
 * - API calls to enable/disable protection
 * - URL updates when OTP state changes
 * - Toast notifications for state changes
 *
 * State updates follow a broadcast pattern: when a user toggles OTP, the API
 * call triggers a server broadcast, and all clients (including the initiator)
 * update their state from the broadcast. This ensures consistency.
 *
 * @param options - Configuration for OTP synchronization
 * @returns OTP state and control functions
 *
 * @throws {ApiError} When OTP API requests fail (error is caught and shown via toast)
 *
 * @example
 * ```tsx
 * const { otpEnabled, toggleOTP, documentUrl } = useOTPSync({
 *   documentId: 'abc123',
 *   currentUserId: 42,
 *   currentUserName: 'Alice',
 *   otpBroadcast
 * });
 *
 * // Toggle OTP protection
 * await toggleOTP(true);
 *
 * // Share the URL (includes OTP if enabled)
 * navigator.clipboard.writeText(documentUrl);
 * ```
 */
export function useOTPSync({
  documentId,
  currentUserId,
  currentUserName,
  otpBroadcast,
}: UseOTPSyncOptions): UseOTPSyncResult {
  const toast = useToast();
  const [otpEnabled, setOtpEnabled] = useState(false);
  const [otp, setOtp] = useState<string | null>(null);
  const [isToggling, setIsToggling] = useState(false);

  // Initialize from URL on mount
  useEffect(() => {
    const otpFromUrl = getOtpFromUrl();
    if (otpFromUrl) {
      setOtp(otpFromUrl);
      setOtpEnabled(true);
      logger.debug('[OTPInit] Loaded OTP from URL');
    } else {
      logger.debug('[OTPInit] No OTP in URL');
    }
  }, []); // Only runs on mount

  // Sync OTP changes from server broadcasts
  useEffect(() => {
    if (otpBroadcast === undefined) {
      logger.debug('[OTPBroadcast] No broadcast yet');
      return;
    }

    logger.debug('[OTPBroadcast] Received OTP change:', {
      otp: otpBroadcast.otp,
      fromUser: otpBroadcast.userId,
      userName: otpBroadcast.userName,
    });

    const isMyChange = otpBroadcast.userId === currentUserId;

    // Update UI state
    if (otpBroadcast.otp) {
      // OTP enabled
      setOtp(otpBroadcast.otp);
      setOtpEnabled(true);
      window.history.replaceState(null, '', `#${documentId}?otp=${otpBroadcast.otp}`);

      toast({
        title: isMyChange ? 'OTP Protection Enabled' : 'OTP Updated',
        description: isMyChange
          ? 'Document is now protected with a secure token'
          : (
            <>
              Document protection has been enabled by <i>{otpBroadcast.userName}</i>
            </>
          ),
        status: 'success',
        duration: UI.TOAST_INFO_DURATION,
        isClosable: true,
      });
    } else {
      // OTP disabled
      setOtp(null);
      setOtpEnabled(false);
      window.history.replaceState(null, '', `#${documentId}`);

      toast({
        title: isMyChange ? 'OTP Protection Disabled' : 'OTP Removed',
        description: isMyChange
          ? 'Document is now accessible without a token'
          : (
            <>
              Document protection has been disabled by <i>{otpBroadcast.userName}</i>
            </>
          ),
        status: 'info',
        duration: UI.TOAST_INFO_DURATION,
        isClosable: true,
      });
    }
  }, [otpBroadcast, currentUserId, documentId, toast]);

  // Toggle OTP protection
  async function toggleOTP(enabled: boolean) {
    logger.debug('[OTPToggle] Starting toggle:', { enabled, documentId });
    setIsToggling(true);
    try {
      if (enabled) {
        // Enable OTP protection
        logger.debug('[OTPToggle] Calling protectDocument API');
        await protectDocument(documentId, currentUserId, currentUserName);
        logger.info('[OTPToggle] Successfully enabled OTP protection');
        // State will update from broadcast
      } else {
        // Disable OTP protection - requires current OTP
        if (!otp) {
          const errorMsg = 'Cannot disable OTP: no OTP available';
          logger.error('[OTPToggle] Error:', errorMsg);
          throw new Error(errorMsg);
        }
        logger.debug('[OTPToggle] Calling unprotectDocument API');
        await unprotectDocument(documentId, currentUserId, currentUserName, otp);
        logger.info('[OTPToggle] Successfully disabled OTP protection');
        // State will update from broadcast
      }
    } catch (error) {
      logger.error('[OTPToggle] Failed to toggle OTP:', error);
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to toggle OTP protection',
        status: 'error',
        duration: UI.TOAST_INFO_DURATION,
        isClosable: true,
      });
      // Revert toggle state on error
      setOtpEnabled(!enabled);
    } finally {
      setIsToggling(false);
    }
  }

  // Generate shareable document URL
  const documentUrl = otp
    ? `${window.location.origin}/#${documentId}?otp=${otp}`
    : `${window.location.origin}/#${documentId}`;

  return {
    otp,
    otpEnabled,
    isToggling,
    documentUrl,
    toggleOTP,
  };
}
