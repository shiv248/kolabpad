/**
 * Tests for useLanguageSync hook
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useLanguageSync } from './useLanguageSync';
import type { LanguageBroadcast } from '../types';
import { USER } from '../constants';

// Mock toast
const mockToast = vi.fn();
vi.mock('@chakra-ui/react', async () => {
  const actual = await vi.importActual('@chakra-ui/react');
  return {
    ...actual,
    useToast: () => mockToast,
  };
});

describe('useLanguageSync', () => {
  const mockOnLanguageChange = vi.fn();

  const defaultProps = {
    languageBroadcast: undefined,
    myUserId: 42,
    onLanguageChange: mockOnLanguageChange,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should do nothing when no broadcast', () => {
    renderHook(() => useLanguageSync(defaultProps));

    expect(mockOnLanguageChange).not.toHaveBeenCalled();
    expect(mockToast).not.toHaveBeenCalled();
  });

  it('should call onLanguageChange when broadcast received', async () => {
    const { rerender } = renderHook(
      ({ languageBroadcast }) => useLanguageSync({ ...defaultProps, languageBroadcast }),
      {
        initialProps: { languageBroadcast: undefined },
      }
    );

    const broadcast: LanguageBroadcast = {
      language: 'javascript',
      userId: 42,
      userName: 'Alice',
    };

    rerender({ languageBroadcast: broadcast });

    await waitFor(() => {
      expect(mockOnLanguageChange).toHaveBeenCalledWith('javascript');
    });
  });

  it('should show toast for own language change', async () => {
    const { rerender } = renderHook(
      ({ languageBroadcast }) => useLanguageSync({ ...defaultProps, languageBroadcast }),
      {
        initialProps: { languageBroadcast: undefined },
      }
    );

    const broadcast: LanguageBroadcast = {
      language: 'python',
      userId: 42, // Same as myUserId
      userName: 'Alice',
    };

    rerender({ languageBroadcast: broadcast });

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Language updated',
          description: 'All users are now editing in python.',
        })
      );
    });
  });

  it('should show toast for other user language change', async () => {
    const { rerender } = renderHook(
      ({ languageBroadcast }) => useLanguageSync({ ...defaultProps, languageBroadcast }),
      {
        initialProps: { languageBroadcast: undefined },
      }
    );

    const broadcast: LanguageBroadcast = {
      language: 'typescript',
      userId: 99, // Different user
      userName: 'Bob',
    };

    rerender({ languageBroadcast: broadcast });

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Language updated',
        })
      );
    });
  });

  it('should not show toast for system/initial state', async () => {
    const { rerender } = renderHook(
      ({ languageBroadcast }) => useLanguageSync({ ...defaultProps, languageBroadcast }),
      {
        initialProps: { languageBroadcast: undefined },
      }
    );

    const broadcast: LanguageBroadcast = {
      language: 'javascript',
      userId: USER.SYSTEM_USER_ID,
      userName: 'System',
    };

    rerender({ languageBroadcast: broadcast });

    await waitFor(() => {
      expect(mockOnLanguageChange).toHaveBeenCalledWith('javascript');
    });

    expect(mockToast).not.toHaveBeenCalled();
  });
});
