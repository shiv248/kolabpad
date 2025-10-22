/**
 * Main sidebar container component
 * Composes all sidebar sub-components into a unified sidebar UI
 */

import { Container, Flex } from '@chakra-ui/react';
import ConnectionStatus from '../shared/ConnectionStatus';
import { DarkModeToggle } from './DarkModeToggle';
import { LanguageSelector } from './LanguageSelector';
import { OTPManager } from './OTPManager';
import { UserList } from './UserList';
import { AboutSection } from './AboutSection';
import { colors, layout } from '../../theme';
import type { UserInfo, OTPBroadcast } from '../../types';

export type SidebarProps = {
  documentId: string;
  connection: 'connected' | 'disconnected' | 'desynchronized';
  darkMode: boolean;
  language: string;
  currentUser: UserInfo & { id: number };
  users: Record<number, UserInfo>;
  onDarkModeChange: () => void;
  onLanguageChange: (language: string) => void;
  onLoadSample: () => void;
  onChangeName: (name: string) => void;
  onChangeColor: () => void;
  otpBroadcast: OTPBroadcast | undefined;
};

function Sidebar({
  documentId,
  connection,
  darkMode,
  language,
  currentUser,
  users,
  onDarkModeChange,
  onLanguageChange,
  onLoadSample,
  onChangeName,
  onChangeColor,
  otpBroadcast,
}: SidebarProps) {
  return (
    <Container
      w={layout.sidebar.width}
      display={{ base: 'none', sm: 'block' }}
      bgColor={darkMode ? colors.dark.bg.secondary : colors.light.bg.secondary}
      overflowY="auto"
      maxW="full"
      lineHeight={1.4}
      py={layout.sidebar.py}
      // ⚠️ Creates stacking context - child popovers use fixed positioning (strategy="fixed")
      position="relative"
    >
      <Flex justifyContent="space-between" alignItems="center" w="full" mb={4}>
        <ConnectionStatus darkMode={darkMode} connection={connection} />
        <DarkModeToggle darkMode={darkMode} onToggle={onDarkModeChange} />
      </Flex>

      <LanguageSelector
        language={language}
        darkMode={darkMode}
        onLanguageChange={onLanguageChange}
      />

      <OTPManager
        documentId={documentId}
        currentUserId={currentUser.id}
        currentUserName={currentUser.name}
        darkMode={darkMode}
        otpBroadcast={otpBroadcast}
      />

      <UserList
        currentUser={currentUser}
        users={users}
        darkMode={darkMode}
        onChangeName={onChangeName}
        onChangeColor={onChangeColor}
      />

      <AboutSection darkMode={darkMode} onLoadSample={onLoadSample} />
    </Container>
  );
}

export default Sidebar;
