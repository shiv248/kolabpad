/**
 * Tests for useOTPSync hook
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useOTPSync } from './useOTPSync';
import * as documentsApi from '../api/documents';
import type { OTPBroadcast } from '../types';

// Mock the API
vi.mock('../api/documents', () => ({
  protectDocument: vi.fn(),
  unprotectDocument: vi.fn(),
}));

// Mock toast
const mockToast = vi.fn();
vi.mock('@chakra-ui/react', async () => {
  const actual = await vi.importActual('@chakra-ui/react');
  return {
    ...actual,
    useToast: () => mockToast,
  };
});

describe('useOTPSync', () => {
  const defaultProps = {
    documentId: 'test-doc-123',
    currentUserId: 42,
    currentUserName: 'Alice',
    otpBroadcast: undefined,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = '';
    window.history.replaceState = vi.fn();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('initialization', () => {
    it('should initialize with OTP from URL', () => {
      window.location.hash = '#test-doc-123?otp=secret123';

      const { result } = renderHook(() => useOTPSync(defaultProps));

      expect(result.current.otp).toBe('secret123');
      expect(result.current.otpEnabled).toBe(true);
    });

    it('should initialize without OTP when not in URL', () => {
      window.location.hash = '#test-doc-123';

      const { result } = renderHook(() => useOTPSync(defaultProps));

      expect(result.current.otp).toBe(null);
      expect(result.current.otpEnabled).toBe(false);
    });
  });

  describe('toggleOTP', () => {
    it('should call protectDocument when enabling OTP', async () => {
      const mockProtect = vi.spyOn(documentsApi, 'protectDocument').mockResolvedValue({ otp: 'new-otp' });

      const { result } = renderHook(() => useOTPSync(defaultProps));

      await result.current.toggleOTP(true);

      expect(mockProtect).toHaveBeenCalledWith('test-doc-123', 42, 'Alice');
    });

    it('should call unprotectDocument when disabling OTP', async () => {
      const mockUnprotect = vi.spyOn(documentsApi, 'unprotectDocument').mockResolvedValue();

      const props = {
        ...defaultProps,
        otpBroadcast: { otp: 'current-otp', userId: 42, userName: 'Alice' } as OTPBroadcast,
      };

      const { result, rerender } = renderHook(() => useOTPSync(props));

      // Wait for broadcast to process
      await waitFor(() => {
        expect(result.current.otp).toBe('current-otp');
      });

      await result.current.toggleOTP(false);

      expect(mockUnprotect).toHaveBeenCalledWith('test-doc-123', 42, 'Alice', 'current-otp');
    });

    it('should show error toast when API fails', async () => {
      const mockProtect = vi.spyOn(documentsApi, 'protectDocument').mockRejectedValue(new Error('API Error'));

      const { result } = renderHook(() => useOTPSync(defaultProps));

      await result.current.toggleOTP(true);

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith(
          expect.objectContaining({
            title: 'Error',
            status: 'error',
          })
        );
      });
    });

    it('should set isToggling during API call', async () => {
      let resolveProtect: (value: any) => void;
      const protectPromise = new Promise((resolve) => {
        resolveProtect = resolve;
      });

      vi.spyOn(documentsApi, 'protectDocument').mockReturnValue(protectPromise as any);

      const { result } = renderHook(() => useOTPSync(defaultProps));

      const togglePromise = result.current.toggleOTP(true);

      // Should be toggling (wait for state update)
      await waitFor(() => {
        expect(result.current.isToggling).toBe(true);
      });

      // Resolve the API call
      resolveProtect!({ otp: 'new-otp' });
      await togglePromise;

      // Should no longer be toggling
      await waitFor(() => {
        expect(result.current.isToggling).toBe(false);
      });
    });
  });

  describe('broadcast synchronization', () => {
    it('should update state when OTP broadcast is received', async () => {
      const { result, rerender } = renderHook(
        ({ otpBroadcast }) => useOTPSync({ ...defaultProps, otpBroadcast }),
        {
          initialProps: { otpBroadcast: undefined },
        }
      );

      const newBroadcast: OTPBroadcast = {
        otp: 'broadcast-otp',
        userId: 99,
        userName: 'Bob',
      };

      rerender({ otpBroadcast: newBroadcast });

      await waitFor(() => {
        expect(result.current.otp).toBe('broadcast-otp');
        expect(result.current.otpEnabled).toBe(true);
      });
    });

    it('should update URL when OTP is enabled via broadcast', async () => {
      const { rerender } = renderHook(
        ({ otpBroadcast }) => useOTPSync({ ...defaultProps, otpBroadcast }),
        {
          initialProps: { otpBroadcast: undefined },
        }
      );

      const newBroadcast: OTPBroadcast = {
        otp: 'broadcast-otp',
        userId: 99,
        userName: 'Bob',
      };

      rerender({ otpBroadcast: newBroadcast });

      await waitFor(() => {
        expect(window.history.replaceState).toHaveBeenCalledWith(
          null,
          '',
          '#test-doc-123?otp=broadcast-otp'
        );
      });
    });

    it('should clear URL when OTP is disabled via broadcast', async () => {
      const { rerender } = renderHook(
        ({ otpBroadcast }) => useOTPSync({ ...defaultProps, otpBroadcast }),
        {
          initialProps: { otpBroadcast: undefined },
        }
      );

      const newBroadcast: OTPBroadcast = {
        otp: null,
        userId: 99,
        userName: 'Bob',
      };

      rerender({ otpBroadcast: newBroadcast });

      await waitFor(() => {
        expect(window.history.replaceState).toHaveBeenCalledWith(null, '', '#test-doc-123');
      });
    });

    it('should show different toast for own changes vs others', async () => {
      const { rerender } = renderHook(
        ({ otpBroadcast }) => useOTPSync({ ...defaultProps, otpBroadcast }),
        {
          initialProps: { otpBroadcast: undefined },
        }
      );

      // Own change
      const ownBroadcast: OTPBroadcast = {
        otp: 'own-otp',
        userId: 42, // Same as currentUserId
        userName: 'Alice',
      };

      rerender({ otpBroadcast: ownBroadcast });

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith(
          expect.objectContaining({
            title: 'OTP Protection Enabled',
            description: 'Document is now protected with a secure token',
          })
        );
      });

      mockToast.mockClear();

      // Other's change
      const otherBroadcast: OTPBroadcast = {
        otp: 'other-otp',
        userId: 99, // Different user
        userName: 'Bob',
      };

      rerender({ otpBroadcast: otherBroadcast });

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith(
          expect.objectContaining({
            title: 'OTP Updated',
          })
        );
      });
    });
  });

  describe('documentUrl', () => {
    it('should generate URL without OTP when disabled', () => {
      const { result } = renderHook(() => useOTPSync(defaultProps));

      expect(result.current.documentUrl).toBe('http://localhost:3000/#test-doc-123');
    });

    it('should generate URL with OTP when enabled', async () => {
      const { result, rerender } = renderHook(
        ({ otpBroadcast }) => useOTPSync({ ...defaultProps, otpBroadcast }),
        {
          initialProps: { otpBroadcast: undefined },
        }
      );

      const broadcast: OTPBroadcast = {
        otp: 'test-otp',
        userId: 42,
        userName: 'Alice',
      };

      rerender({ otpBroadcast: broadcast });

      await waitFor(() => {
        expect(result.current.documentUrl).toBe('http://localhost:3000/#test-doc-123?otp=test-otp');
      });
    });
  });
});
