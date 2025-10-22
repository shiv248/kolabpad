/**
 * Tests for useColorCollision hook
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useColorCollision } from './useColorCollision';
import type { UserInfo } from '../types';

describe('useColorCollision', () => {
  const mockOnHueChange = vi.fn();

  const defaultProps = {
    connection: 'disconnected' as const,
    myUserId: -1,
    users: {},
    currentHue: 100,
    onHueChange: mockOnHueChange,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should not check for collisions when not connected', () => {
    renderHook(() => useColorCollision(defaultProps));

    expect(mockOnHueChange).not.toHaveBeenCalled();
  });

  it('should not check for collisions when user ID not assigned', () => {
    renderHook(() =>
      useColorCollision({
        ...defaultProps,
        connection: 'connected',
      })
    );

    expect(mockOnHueChange).not.toHaveBeenCalled();
  });

  it('should not change hue when no collision', () => {
    const users: Record<number, UserInfo> = {
      10: { name: 'Alice', hue: 200 }, // Far away
      20: { name: 'Bob', hue: 300 }, // Far away
    };

    renderHook(() =>
      useColorCollision({
        ...defaultProps,
        connection: 'connected',
        myUserId: 5,
        users,
        currentHue: 100,
      })
    );

    expect(mockOnHueChange).not.toHaveBeenCalled();
  });

  it('should change hue when collision detected', async () => {
    const users: Record<number, UserInfo> = {
      10: { name: 'Alice', hue: 105 }, // Collision! (within 15-degree threshold)
      20: { name: 'Bob', hue: 200 },
    };

    renderHook(() =>
      useColorCollision({
        ...defaultProps,
        connection: 'connected',
        myUserId: 5,
        users,
        currentHue: 100,
      })
    );

    await waitFor(() => {
      expect(mockOnHueChange).toHaveBeenCalledTimes(1);
    });

    // Should be called with a new hue that doesn't collide
    const newHue = mockOnHueChange.mock.calls[0][0];
    expect(newHue).toBeGreaterThanOrEqual(0);
    expect(newHue).toBeLessThan(360);
  });

  it('should only check once per session (ref check)', () => {
    const users: Record<number, UserInfo> = {
      10: { name: 'Alice', hue: 105 },
    };

    const { rerender } = renderHook(
      (props) => useColorCollision(props),
      {
        initialProps: {
          ...defaultProps,
          connection: 'connected' as const,
          myUserId: 5,
          users,
          currentHue: 100,
        },
      }
    );

    // Should call onHueChange once
    expect(mockOnHueChange).toHaveBeenCalledTimes(1);

    // Rerender with same props
    rerender({
      ...defaultProps,
      connection: 'connected' as const,
      myUserId: 5,
      users,
      currentHue: 100,
    });

    // Should still only be called once (ref prevents re-checking)
    expect(mockOnHueChange).toHaveBeenCalledTimes(1);
  });

  it('should exclude self from collision check', () => {
    const users: Record<number, UserInfo> = {
      5: { name: 'Me', hue: 100 }, // Same hue, same ID (self)
      10: { name: 'Alice', hue: 200 },
    };

    renderHook(() =>
      useColorCollision({
        ...defaultProps,
        connection: 'connected',
        myUserId: 5,
        users,
        currentHue: 100,
      })
    );

    // Should not change hue (ignores self)
    expect(mockOnHueChange).not.toHaveBeenCalled();
  });
});
