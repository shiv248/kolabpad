/**
 * Dark mode toggle button component
 */

import { IconButton } from '@chakra-ui/react';
import { IoMoon, IoSunny } from 'react-icons/io5';
import { colors } from '../../theme';

export interface DarkModeToggleProps {
  darkMode: boolean;
  onToggle: () => void;
}

export function DarkModeToggle({ darkMode, onToggle }: DarkModeToggleProps) {
  return (
    <IconButton
      aria-label="Toggle dark mode"
      icon={darkMode ? <IoSunny /> : <IoMoon />}
      onClick={onToggle}
      size="sm"
      variant="ghost"
      color={darkMode ? 'yellow.300' : 'gray.700'}
      border="1px solid"
      borderColor={darkMode ? 'whiteAlpha.400' : 'blackAlpha.300'}
      _hover={{
        bg: darkMode ? 'whiteAlpha.200' : 'blackAlpha.100',
        borderColor: darkMode ? 'whiteAlpha.600' : 'blackAlpha.500',
      }}
    />
  );
}
