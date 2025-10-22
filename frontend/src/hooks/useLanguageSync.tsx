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
  myUserId: number | null;
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

  // Only trigger when languageBroadcast actually changes (not when myUserId changes)
  useEffect(() => {
    if (languageBroadcast === undefined) {
      logger.debug('[LanguageBroadcast] No broadcast yet');
      return;
    }

    const isMyChange = languageBroadcast.userId === myUserId;
    const isInitialState = languageBroadcast.userId === USER.SYSTEM_USER_ID;

    logger.debug('[LanguageBroadcast] Received language change:', {
      language: languageBroadcast.language,
      fromUser: languageBroadcast.userId,
      userName: languageBroadcast.userName,
      isMyChange,
      isInitialState,
    });

    // Update language state via callback
    onLanguageChange(languageBroadcast.language);
    logger.info('[LanguageBroadcast] Language updated to:', languageBroadcast.language);

    // Show appropriate toast (skip for initial state)
    if (!isInitialState) {
      if (isMyChange) {
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
    }
    // Note: myUserId is used inside but not a dependency - we read latest value
    // We only want to run when languageBroadcast changes, not when myUserId changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [languageBroadcast, toast, onLanguageChange]);
}
