import {
  Button,
  Container,
  Flex,
  Heading,
  IconButton,
  Input,
  InputGroup,
  InputRightElement,
  Link,
  Select,
  Stack,
  Text,
  useToast,
} from "@chakra-ui/react";
import { IoMoon, IoSunny } from "react-icons/io5";
import { VscRepo } from "react-icons/vsc";

import ConnectionStatus from "./ConnectionStatus";
import { colors, layout } from "./theme";
import User from "./User";
import languages from "./languages.json";
import type { UserInfo } from "./kolabpad";

export type SidebarProps = {
  documentId: string;
  connection: "connected" | "disconnected" | "desynchronized";
  darkMode: boolean;
  language: string;
  currentUser: UserInfo;
  users: Record<number, UserInfo>;
  onDarkModeChange: () => void;
  onLanguageChange: (language: string) => void;
  onLoadSample: () => void;
  onChangeName: (name: string) => void;
  onChangeColor: () => void;
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
}: SidebarProps) {
  const toast = useToast();

  // For sharing the document by link to others.
  const documentUrl = `${window.location.origin}/#${documentId}`;

  async function handleCopy() {
    await navigator.clipboard.writeText(documentUrl);
    toast({
      title: "Copied!",
      description: "Link copied to clipboard",
      status: "success",
      duration: 2000,
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
          <option key={lang} value={lang} style={{ color: "black" }}>
            {lang}
          </option>
        ))}
      </Select>

      <Heading mt={4} mb={1.5} size="sm">
        Share Link
      </Heading>
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
            Copy
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
