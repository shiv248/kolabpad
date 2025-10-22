import {
  Button,
  ButtonGroup,
  HStack,
  Icon,
  Input,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverCloseButton,
  PopoverContent,
  PopoverFooter,
  PopoverHeader,
  PopoverTrigger,
  Text,
  useDisclosure,
} from "@chakra-ui/react";
import { useRef, useState, useEffect } from "react";
import { FaPalette } from "react-icons/fa";
import { VscAccount } from "react-icons/vsc";

import { USER } from "../../constants";
import type { UserInfo } from "../../types";
import { colors, zIndex } from "../../theme";

type UserProps = {
  info: UserInfo;
  isMe?: boolean;
  onChangeName?: (name: string) => void;
  onChangeColor?: () => void;
  darkMode: boolean;
};

function User({
  info,
  isMe = false,
  onChangeName,
  onChangeColor,
  darkMode,
}: UserProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const { isOpen, onOpen, onClose } = useDisclosure();

  // Local state for name input - only commit to parent on "Done"
  const [localName, setLocalName] = useState(info.name);

  // Sync local state when prop changes (e.g., from broadcasts)
  useEffect(() => {
    setLocalName(info.name);
  }, [info.name]);

  // Handle Done button click - validate and commit changes
  const handleDone = () => {
    const trimmedName = localName.trim();

    // Validate: name must not be empty
    if (trimmedName.length === 0) {
      // Reset to original name if empty
      setLocalName(info.name);
      onClose();
      return;
    }

    // Only broadcast if name actually changed
    if (trimmedName !== info.name) {
      onChangeName?.(trimmedName);
    }

    onClose();
  };

  const nameColor = `hsl(${info.hue}, 90%, ${darkMode ? "70%" : "25%"})`;
  return (
    <Popover
      placement="right"
      isOpen={isOpen}
      onClose={onClose}
      initialFocusRef={inputRef}
      strategy="fixed"
    >
      <PopoverTrigger>
        <HStack
          p={2}
          rounded="md"
          _hover={{
            bgColor: darkMode ? colors.dark.bg.hover : colors.light.bg.hover,
            cursor: "pointer",
          }}
          onClick={() => isMe && onOpen()}
        >
          <Icon as={VscAccount} />
          <Text fontWeight="medium" color={nameColor}>
            {info.name}
          </Text>
          {isMe && <Text>(you)</Text>}
        </HStack>
      </PopoverTrigger>
      <PopoverContent
        bgColor={darkMode ? colors.dark.bg.tertiary : colors.light.bg.elevated}
        borderColor={darkMode ? colors.dark.border : colors.light.border}
        zIndex={zIndex.popover}
      >
        <PopoverHeader
          fontWeight="semibold"
          borderColor={darkMode ? colors.dark.border : colors.light.border}
        >
          Update Info
        </PopoverHeader>
        <PopoverArrow bgColor={darkMode ? colors.dark.bg.tertiary : colors.light.bg.elevated} />
        <PopoverCloseButton />
        <PopoverBody borderColor={darkMode ? colors.dark.border : colors.light.border}>
          <Input
            ref={inputRef}
            mb={2}
            value={localName}
            maxLength={USER.MAX_NAME_LENGTH}
            onChange={(event) => setLocalName(event.target.value)}
          />
          <Button
            size="sm"
            w="100%"
            leftIcon={<FaPalette />}
            colorScheme={darkMode ? "whiteAlpha" : "gray"}
            onClick={onChangeColor}
          >
            Change Color
          </Button>
        </PopoverBody>
        <PopoverFooter
          display="flex"
          justifyContent="flex-end"
          borderColor={darkMode ? colors.dark.border : colors.light.border}
        >
          <ButtonGroup size="sm">
            <Button colorScheme="blue" onClick={handleDone}>
              Done
            </Button>
          </ButtonGroup>
        </PopoverFooter>
      </PopoverContent>
    </Popover>
  );
}

export default User;
