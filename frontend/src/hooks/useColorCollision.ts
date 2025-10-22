/**
 * Custom hook for detecting and resolving color collisions when joining a document
 * Automatically adjusts user hue if it collides with existing users
 */

import { useEffect, useRef } from 'react';
import { generateHue, hasHueCollision } from '../utils/color';
import { logger } from '../logger';
import type { UserInfo } from '../types';

export interface UseColorCollisionOptions {
  connection: 'connected' | 'disconnected' | 'desynchronized';
  myUserId: number;
  users: Record<number, UserInfo>;
  currentHue: number;
  onHueChange: (newHue: number) => void;
}

/**
 * Hook to detect color collisions and auto-adjust hue when joining
 * Only checks once per document session when initially connected
 */
export function useColorCollision({
  connection,
  myUserId,
  users,
  currentHue,
  onHueChange,
}: UseColorCollisionOptions): void {
  const collisionCheckDoneRef = useRef(false);

  useEffect(() => {
    // Only check once when initially connected
    if (connection !== 'connected' || myUserId === -1 || collisionCheckDoneRef.current) {
      return;
    }

    // Extract hues from other users (excluding self)
    const existingHues = Object.entries(users)
      .filter(([id]) => Number(id) !== myUserId)
      .map(([, user]) => user.hue);

    // Check if current hue collides with existing users
    if (existingHues.length > 0 && hasHueCollision(currentHue, existingHues)) {
      const newHue = generateHue(existingHues);
      logger.debug('[Color] Collision detected on join, changing hue from %d to %d', currentHue, newHue);
      onHueChange(newHue);
    }

    // Mark collision check as done for this document session
    collisionCheckDoneRef.current = true;
  }, [connection, myUserId, users, currentHue, onHueChange]);
}
