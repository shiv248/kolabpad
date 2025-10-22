import { Flex, Icon, Text } from "@chakra-ui/react";
import { useState } from "react";
import { VscFileCode } from "react-icons/vsc";
import { colors, layout } from "../../theme";

const version =
  import.meta.env.VITE_SHA && import.meta.env.VITE_SHA !== "undefined"
    ? import.meta.env.VITE_SHA.slice(0, 7)
    : "Development";

function Footer() {
  const [showVersion, setShowVersion] = useState(false);

  return (
    <Flex h={layout.footer.height} bgColor={colors.dark.bg.footer} color={colors.dark.text.footer}>
      <Flex
        h="100%"
        bgColor={colors.dark.bg.footerAccent}
        pl={2.5}
        pr={4}
        fontSize={layout.footer.fontSize}
        align="center"
        cursor="pointer"
        onClick={() => setShowVersion(!showVersion)}
        _hover={{ bgColor: colors.dark.bg.footerAccentHover }}
      >
        <Icon as={VscFileCode} mb={-0.5} mr={1} />
        <Text fontSize="xs">
          Kolabpad ({showVersion ? `v: ${version}` : "Document"})
        </Text>
      </Flex>
    </Flex>
  );
}

export default Footer;
