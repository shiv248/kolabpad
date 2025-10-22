import {
  Button,
  Container,
  Flex,
  FormControl,
  FormLabel,
  Heading,
  IconButton,
  Input,
  InputGroup,
  InputRightElement,
  Link,
  Select,
  Stack,
  Switch,
  Text,
  useToast,
} from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { IoMoon, IoSunny } from "react-icons/io5";
import { VscRepo } from "react-icons/vsc";
import { UI } from "./constants";
import { logger } from "./logger";
import { getOtpFromUrl } from "./utils/url";

import ConnectionStatus from "./ConnectionStatus";
import { colors, layout } from "./theme";
import User from "./User";
import languages from "./languages.json";
import type { UserInfo } from "./kolabpad";

type OTPBroadcast = {
  otp: string | null;
  userId: number;
  userName: string;
};

export type SidebarProps = {
  documentId: string;
  connection: "connected" | "disconnected" | "desynchronized";
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
  const toast = useToast();
  const [otpEnabled, setOtpEnabled] = useState(false);
  const [otp, setOtp] = useState<string | null>(null);
  const [isToggling, setIsToggling] = useState(false);
  const [copied, setCopied] = useState(false);

  // Check if OTP is in the URL on component mount
  // Note: Component remounts completely when documentId changes (via key prop),
  // so this effect runs fresh for each document
  useEffect(() => {
    const otpFromUrl = getOtpFromUrl();
    if (otpFromUrl) {
      setOtp(otpFromUrl);
      setOtpEnabled(true);
      logger.debug('[OTPInit] Loaded OTP from URL');
    } else {
      logger.debug('[OTPInit] No OTP in URL');
    }
  }, []); // Only runs on mount (component remounts when document changes)

  // Sync OTP changes from server broadcasts
  // All clients (including the one who initiated the change) update from broadcasts
  useEffect(() => {
    // Skip if no broadcast received yet
    if (otpBroadcast === undefined) {
      logger.debug('[OTPBroadcast] No broadcast yet');
      return;
    }

    logger.debug('[OTPBroadcast] Received OTP change:', {
      otp: otpBroadcast.otp,
      fromUser: otpBroadcast.userId,
      userName: otpBroadcast.userName
    });

    // Determine if this is my change
    const isMyChange = otpBroadcast.userId === currentUser.id;

    // Update UI state (same for everyone)
    if (otpBroadcast.otp) {
      // OTP enabled
      setOtp(otpBroadcast.otp);
      setOtpEnabled(true);
      window.history.replaceState(null, "", `#${documentId}?otp=${otpBroadcast.otp}`);

      // Show appropriate toast
      if (isMyChange) {
        toast({
          title: "OTP Protection Enabled",
          description: "Document is now protected with a secure token",
          status: "success",
          duration: UI.TOAST_INFO_DURATION,
          isClosable: true,
        });
      } else {
        toast({
          title: "OTP Updated",
          description: (
            <>
              Document protection has been enabled by <i>{otpBroadcast.userName}</i>
            </>
          ),
          status: "success",
          duration: UI.TOAST_INFO_DURATION,
          isClosable: true,
        });
      }
    } else {
      // OTP disabled
      setOtp(null);
      setOtpEnabled(false);
      window.history.replaceState(null, "", `#${documentId}`);

      // Show appropriate toast
      if (isMyChange) {
        toast({
          title: "OTP Protection Disabled",
          description: "Document is now accessible without a token",
          status: "info",
          duration: UI.TOAST_INFO_DURATION,
          isClosable: true,
        });
      } else {
        toast({
          title: "OTP Removed",
          description: (
            <>
              Document protection has been disabled by <i>{otpBroadcast.userName}</i>
            </>
          ),
          status: "info",
          duration: UI.TOAST_INFO_DURATION,
          isClosable: true,
        });
      }
    }
  }, [otpBroadcast, currentUser.id, documentId, toast]);

  // For sharing the document by link to others.
  const documentUrl = otp
    ? `${window.location.origin}/#${documentId}?otp=${otp}`
    : `${window.location.origin}/#${documentId}`;

  async function handleOtpToggle(checked: boolean) {
    setIsToggling(true);
    try {
      if (checked) {
        // Enable OTP protection
        const response = await fetch(`/api/document/${documentId}/protect`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            user_id: currentUser.id,
            user_name: currentUser.name,
          }),
        });
        if (!response.ok) {
          throw new Error("Failed to enable OTP protection");
        }
        // Don't update local state - wait for broadcast
      } else {
        // Disable OTP protection - MUST provide current OTP for security
        const response = await fetch(`/api/document/${documentId}/protect`, {
          method: "DELETE",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            user_id: currentUser.id,
            user_name: currentUser.name,
            otp: otp, // Current OTP required
          }),
        });
        if (!response.ok) {
          throw new Error("Failed to disable OTP protection");
        }
        // Don't update local state - wait for broadcast
      }
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to toggle OTP protection",
        status: "error",
        duration: UI.TOAST_INFO_DURATION,
        isClosable: true,
      });
      // Revert the toggle state on error
      setOtpEnabled(!checked);
    } finally {
      setIsToggling(false);
    }
  }

  async function handleCopy() {
    await navigator.clipboard.writeText(documentUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), UI.COPY_FEEDBACK_DURATION);
    toast({
      title: "Copied!",
      description: "Link copied to clipboard",
      status: "success",
      duration: UI.TOAST_SUCCESS_DURATION,
      isClosable: true,
    });
  }

  return (
    <Container
      w={layout.sidebar.width}
      display={{ base: "none", sm: "block" }}
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
        <IconButton
          aria-label="Toggle dark mode"
          icon={darkMode ? <IoSunny /> : <IoMoon />}
          onClick={onDarkModeChange}
          size="sm"
          variant="ghost"
          color={darkMode ? "yellow.300" : "gray.700"}
          border="1px solid"
          borderColor={darkMode ? "whiteAlpha.400" : "blackAlpha.300"}
          _hover={{
            bg: darkMode ? "whiteAlpha.200" : "blackAlpha.100",
            borderColor: darkMode ? "whiteAlpha.600" : "blackAlpha.500",
          }}
        />
      </Flex>

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
          onChange={(e) => handleOtpToggle(e.target.checked)}
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
            color={darkMode ? "white" : "inherit"}
          >
            {copied ? "Copied!" : "Copy"}
          </Button>
        </InputRightElement>
      </InputGroup>

      <Heading mt={4} mb={1.5} size="sm">
        Active Users
      </Heading>
      <Stack spacing={0} mb={1.5} fontSize="sm">
        <User
          info={currentUser}
          isMe
          onChangeName={onChangeName}
          onChangeColor={onChangeColor}
          darkMode={darkMode}
        />
        {Object.entries(users).map(([id, info]) => (
          <User key={id} info={info} darkMode={darkMode} />
        ))}
      </Stack>

      <Heading mt={4} mb={1.5} size="sm">
        About
      </Heading>
      <Text fontSize="sm" mb={1.5}>
        <strong>Kolabpad</strong> is an open-source collaborative text editor
        based on the{" "}
        <Link
          color={darkMode ? colors.dark.accent.link : colors.light.accent.link}
          fontWeight="semibold"
          href="http://github.com/shiv248/operational-transformation-go"
          isExternal
        >
          Operational Transformation
        </Link>{" "}
        algorithm.
      </Text>
      <Text fontSize="sm" mb={1.5}>
        Share a link to this pad with others, and they can edit from their
        browser while seeing your changes in real time.
      </Text>
      <Text fontSize="sm" mb={1.5}>
        Built using Golang and TypeScript. See the{" "}
        <Link
          color={darkMode ? colors.dark.accent.link : colors.light.accent.link}
          fontWeight="semibold"
          href="http://github.com/shiv248/kolabpad"
          isExternal
        >
          GitHub repository
        </Link>{" "}
        for details.
      </Text>

      <Button
        size="sm"
        colorScheme={darkMode ? "whiteAlpha" : "blackAlpha"}
        borderColor={darkMode ? colors.dark.accent.documentIcon : colors.light.accent.documentIcon}
        color={darkMode ? colors.dark.accent.documentIcon : colors.light.accent.documentIcon}
        variant="outline"
        leftIcon={<VscRepo />}
        mt={1}
        onClick={onLoadSample}
      >
        Read the code
      </Button>
    </Container>
  );
}

export default Sidebar;
