/**
 * About section component with project description and links
 */

import { Button, Heading, Link, Text } from '@chakra-ui/react';
import { VscRepo } from 'react-icons/vsc';
import { colors } from '../../theme';

export interface AboutSectionProps {
  darkMode: boolean;
  onLoadSample: () => void;
}

export function AboutSection({ darkMode, onLoadSample }: AboutSectionProps) {
  return (
    <>
      <Heading mt={4} mb={1.5} size="sm">
        About
      </Heading>
      <Text fontSize="sm" mb={1.5}>
        <strong>Kolabpad</strong> is an open-source collaborative text editor
        based on the{' '}
        <Link
          color={darkMode ? colors.dark.accent.link : colors.light.accent.link}
          fontWeight="semibold"
          href="http://github.com/shiv248/operational-transformation-go"
          isExternal
        >
          Operational Transformation
        </Link>{' '}
        algorithm.
      </Text>
      <Text fontSize="sm" mb={1.5}>
        Share a link to this pad with others, and they can edit from their
        browser while seeing your changes in real time.
      </Text>
      <Text fontSize="sm" mb={1.5}>
        Built using Golang and TypeScript. See the{' '}
        <Link
          color={darkMode ? colors.dark.accent.link : colors.light.accent.link}
          fontWeight="semibold"
          href="http://github.com/shiv248/kolabpad"
          isExternal
        >
          GitHub repository
        </Link>{' '}
        for details.
      </Text>

      <Button
        size="sm"
        colorScheme={darkMode ? 'whiteAlpha' : 'blackAlpha'}
        borderColor={darkMode ? colors.dark.accent.documentIcon : colors.light.accent.documentIcon}
        color={darkMode ? colors.dark.accent.documentIcon : colors.light.accent.documentIcon}
        variant="outline"
        leftIcon={<VscRepo />}
        mt={1}
        onClick={onLoadSample}
      >
        Read the code
      </Button>
    </>
  );
}
