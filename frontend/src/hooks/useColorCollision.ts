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
      logger.debug('[ColorCollision] Skipping check:', {
        connection,
        myUserId,
        alreadyChecked: collisionCheckDoneRef.current,
      });
      return;
    }

    logger.debug('[ColorCollision] Running collision check on join');

    // Extract hues from other users (excluding self)
    const existingHues = Object.entries(users)
      .filter(([id]) => Number(id) !== myUserId)
      .map(([, user]) => user.hue);

    logger.debug('[ColorCollision] Existing hues:', existingHues);

    // Check if current hue collides with existing users
    if (existingHues.length > 0 && hasHueCollision(currentHue, existingHues)) {
      const newHue = generateHue(existingHues);
      logger.info('[ColorCollision] Collision detected, changing hue from %d to %d', currentHue, newHue);
      onHueChange(newHue);
    } else {
      logger.debug('[ColorCollision] No collision detected, keeping hue:', currentHue);
    }

    // Mark collision check as done for this document session
    collisionCheckDoneRef.current = true;
    logger.debug('[ColorCollision] Check completed, marked as done');
  }, [connection, myUserId, users, currentHue, onHueChange]);
}
