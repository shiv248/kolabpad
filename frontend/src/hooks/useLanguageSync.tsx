/**
 * Custom hook for managing language broadcast synchronization
 * Handles toast notifications when language changes via server broadcasts
 */

import { useEffect } from 'react';
import { useToast } from '@chakra-ui/react';
import { logger } from '../logger';
import { USER } from '../constants';
import type { LanguageBroadcast } from '../types';

export interface UseLanguageSyncOptions {
  languageBroadcast: LanguageBroadcast | undefined;
  myUserId: number;
  onLanguageChange: (language: string) => void;
}

/**
 * Hook to handle language broadcast synchronization and notifications
 * Updates language state and shows appropriate toasts based on who changed it
 */
export function useLanguageSync({
  languageBroadcast,
  myUserId,
  onLanguageChange,
}: UseLanguageSyncOptions): void {
  const toast = useToast();

  useEffect(() => {
    if (languageBroadcast === undefined) return;

    const isMyChange = languageBroadcast.userId === myUserId;
    const isInitialState = languageBroadcast.userId === USER.SYSTEM_USER_ID;

    // Update language state via callback
    onLanguageChange(languageBroadcast.language);

    // Show appropriate toast (skip for initial state)
    if (isInitialState) {
      logger.debug('[Language] Initial state received, no toast');
    } else if (isMyChange) {
      toast({
        title: 'Language updated',
        description: `All users are now editing in ${languageBroadcast.language}.`,
        status: 'info',
        duration: 2000,
        isClosable: true,
      });
    } else {
      toast({
        title: 'Language updated',
        description: (
          <>
            Language changed to {languageBroadcast.language} by <i>{languageBroadcast.userName}</i>
          </>
        ),
        status: 'info',
        duration: 2000,
        isClosable: true,
      });
    }
  }, [languageBroadcast, myUserId, toast, onLanguageChange]);
}
