import { Flex, Icon, Text } from "@chakra-ui/react";
import { useState } from "react";
import { VscFileCode } from "react-icons/vsc";

const version =
  import.meta.env.VITE_SHA && import.meta.env.VITE_SHA !== "undefined"
    ? import.meta.env.VITE_SHA.slice(0, 7)
    : "Development";

function Footer() {
  const [showVersion, setShowVersion] = useState(false);

  return (
    <Flex h="22px" bgColor="#0071c3" color="white">
      <Flex
        h="100%"
        bgColor="#09835c"
        pl={2.5}
        pr={4}
        fontSize="sm"
        align="center"
        cursor="pointer"
        onClick={() => setShowVersion(!showVersion)}
        _hover={{ bgColor: "#0a9668" }}
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
