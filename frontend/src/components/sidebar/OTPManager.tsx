/**
 * OTP protection manager component
 * Handles OTP toggle, share link display, and clipboard copy
 */

import {
  Button,
  FormControl,
  FormLabel,
  Heading,
  Input,
  InputGroup,
  InputRightElement,
  Switch,
} from '@chakra-ui/react';
import { useState } from 'react';
import { useToast } from '@chakra-ui/react';
import { useOTPSync } from '../../hooks/useOTPSync';
import { colors } from '../../theme';
import { UI } from '../../constants';
import type { OTPBroadcast } from '../../types';

export interface OTPManagerProps {
  documentId: string;
  currentUserId: number;
  currentUserName: string;
  darkMode: boolean;
  otpBroadcast: OTPBroadcast | undefined;
}

export function OTPManager({
  documentId,
  currentUserId,
  currentUserName,
  darkMode,
  otpBroadcast,
}: OTPManagerProps) {
  const toast = useToast();
  const [copied, setCopied] = useState(false);

  const { otpEnabled, isToggling, documentUrl, toggleOTP } = useOTPSync({
    documentId,
    currentUserId,
    currentUserName,
    otpBroadcast,
  });

  async function handleCopy() {
    await navigator.clipboard.writeText(documentUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), UI.COPY_FEEDBACK_DURATION);
    toast({
      title: 'Copied!',
      description: 'Link copied to clipboard',
      status: 'success',
      duration: UI.TOAST_SUCCESS_DURATION,
      isClosable: true,
    });
  }

  return (
    <>
      <Heading mt={4} mb={1.5} size="sm">
        Share Link
      </Heading>
      <FormControl display="flex" alignItems="center" mb={2}>
        <FormLabel htmlFor="otp-toggle" mb="0" fontSize="sm">
          OTP
        </FormLabel>
        <Switch
          id="otp-toggle"
          isChecked={otpEnabled}
          onChange={(e) => toggleOTP(e.target.checked)}
          isDisabled={isToggling}
          colorScheme="blue"
          size="sm"
        />
      </FormControl>
      <InputGroup size="sm">
        <Input
          readOnly
          pr="3.5rem"
          variant="outline"
          bgColor={darkMode ? colors.dark.bg.elevated : colors.light.bg.elevated}
          borderColor={darkMode ? colors.dark.bg.elevated : colors.light.bg.elevated}
          value={documentUrl}
        />
        <InputRightElement width="3.5rem">
          <Button
            h="1.4rem"
            size="xs"
            onClick={handleCopy}
            _hover={{ bg: darkMode ? colors.dark.bg.hover : colors.light.bg.hover }}
            bgColor={darkMode ? colors.dark.bg.hover : colors.light.bg.hover}
            color={darkMode ? 'white' : 'inherit'}
          >
            {copied ? 'Copied!' : 'Copy'}
          </Button>
        </InputRightElement>
      </InputGroup>
    </>
  );
}
