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

export interface UseOTPSyncOptions {
  documentId: string;
  currentUserId: number;
  currentUserName: string;
  otpBroadcast: OTPBroadcast | undefined;
}

export interface UseOTPSyncResult {
  otp: string | null;
  otpEnabled: boolean;
  isToggling: boolean;
  documentUrl: string;
  toggleOTP: (enabled: boolean) => Promise<void>;
}

/**
 * Hook to manage OTP protection state with broadcast synchronization
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
    setIsToggling(true);
    try {
      if (enabled) {
        // Enable OTP protection
        await protectDocument(documentId, currentUserId, currentUserName);
        // State will update from broadcast
      } else {
        // Disable OTP protection - requires current OTP
        if (!otp) {
          throw new Error('Cannot disable OTP: no OTP available');
        }
        await unprotectDocument(documentId, currentUserId, currentUserName, otp);
        // State will update from broadcast
      }
    } catch (error) {
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
