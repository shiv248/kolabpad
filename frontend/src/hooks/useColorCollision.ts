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
  myUserId: number | null;
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
    if (connection !== 'connected' || myUserId === null || collisionCheckDoneRef.current) {
      // Don't log if we're just waiting for user ID assignment (expected flow)
      if (connection === 'connected' && myUserId === null && !collisionCheckDoneRef.current) {
        logger.debug('[ColorCollision] Waiting for user ID assignment');
      }
      return;
    }

    logger.info('[ColorCollision] Running collision check on join');

    // Extract hues from other users (excluding self)
    const existingHues = Object.entries(users)
      .filter(([id]) => Number(id) !== myUserId)
      .map(([, user]) => user.hue);

    // Check if current hue collides with existing users
    if (existingHues.length > 0 && hasHueCollision(currentHue, existingHues)) {
      const newHue = generateHue(existingHues);
      logger.info('[ColorCollision] Collision detected, changing hue: %d → %d', currentHue, newHue);
      onHueChange(newHue);
    } else {
      logger.debug('[ColorCollision] No collision detected (existing users: %d)', existingHues.length);
    }

    // Mark collision check as done for this document session
    collisionCheckDoneRef.current = true;
    // onHueChange is stable (setState from useLocalStorageState) - no need as dependency
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connection, myUserId, users, currentHue]);
}
