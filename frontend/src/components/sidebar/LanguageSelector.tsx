/**
 * Language selector dropdown component
 */

import { Heading, Select } from '@chakra-ui/react';
import languages from '../../languages.json';
import { colors } from '../../theme';

export interface LanguageSelectorProps {
  language: string;
  darkMode: boolean;
  onLanguageChange: (language: string) => void;
}

export function LanguageSelector({
  language,
  darkMode,
  onLanguageChange,
}: LanguageSelectorProps) {
  return (
    <>
      <Heading mt={4} mb={1.5} size="sm">
        Language
      </Heading>
      <Select
        size="sm"
        bgColor={darkMode ? colors.dark.bg.elevated : colors.light.bg.elevated}
        borderColor={darkMode ? colors.dark.bg.elevated : colors.light.bg.elevated}
        value={language}
        onChange={(event) => onLanguageChange(event.target.value)}
      >
        {languages.map((lang) => (
          <option key={lang} value={lang}>
            {lang}
          </option>
        ))}
      </Select>
    </>
  );
}
