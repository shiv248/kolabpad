import { Box, Flex, HStack, Icon, Text, useToast } from "@chakra-ui/react";
import Editor from "@monaco-editor/react";
import { editor } from "monaco-editor/esm/vs/editor/editor.api";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api";
import { useEffect, useRef, useState } from "react";
import { VscChevronRight, VscFolderOpened, VscGist } from "react-icons/vsc";
import useLocalStorageState from "use-local-storage-state";

import kolabpadRaw from "../../pkg/server/kolabpad.go?raw";
import AuthBlockedDialog from "./AuthBlockedDialog";
import Footer from "./Footer";
import ReadCodeConfirm from "./ReadCodeConfirm";
import Sidebar from "./Sidebar";
import animals from "./animals.json";
import languages from "./languages.json";
import Kolabpad, { UserInfo } from "./kolabpad";
import { colors, layout } from "./theme";
import useHash from "./useHash";

function getWsUri(id: string) {
  let url = new URL(`api/socket/${id}`, window.location.href);
  url.protocol = url.protocol == "https:" ? "wss:" : "ws:";

  // Add OTP parameter if present in the URL
  const hashParts = window.location.hash.slice(1).split('?');
  const params = new URLSearchParams(hashParts[1] || '');
  const otp = params.get('otp');
  if (otp) {
    url.searchParams.set('otp', otp);
  }

  return url.href;
}

function generateName() {
  return "Anonymous " + animals[Math.floor(Math.random() * animals.length)];
}

function generateHue() {
  return Math.floor(Math.random() * 360);
}

/**
 * Comprehensively resets Monaco editor state when switching documents.
 * Prevents state leakage between documents (undo history, decorations, etc.)
 */
function resetEditorState(editor: editor.IStandaloneCodeEditor) {
  const model = editor.getModel()!;

  // Clear content and set line ending
  model.setValue("");
  model.setEOL(0); // LF

  // Clear undo/redo stack by pushing empty edit operations
  model.pushEditOperations([], [], () => null);

  // Clear all decorations
  const decorations = model.getAllDecorations();
  if (decorations.length > 0) {
    model.deltaDecorations(decorations.map(d => d.id), []);
  }

  // Reset view state (scroll position, cursor position)
  editor.setScrollPosition({ scrollTop: 0, scrollLeft: 0 });
  editor.setPosition({ column: 1, lineNumber: 1 });

  // Clear all markers (errors, warnings, etc.)
  monaco.editor.setModelMarkers(model, 'owner', []);

  // Reset selection
  editor.setSelection({
    startLineNumber: 1,
    startColumn: 1,
    endLineNumber: 1,
    endColumn: 1,
  });
}

function App() {
  const toast = useToast();
  const [language, setLanguage] = useState("plaintext");
  const [connection, setConnection] = useState<
    "connected" | "disconnected" | "desynchronized"
  >("disconnected");
  const [users, setUsers] = useState<Record<number, UserInfo>>({});
  const [name, setName] = useLocalStorageState("name", {
    defaultValue: generateName,
  });
  const [hue, setHue] = useLocalStorageState("hue", {
    defaultValue: generateHue,
  });
  const [editor, setEditor] = useState<editor.IStandaloneCodeEditor>();
  const [darkMode, setDarkMode] = useLocalStorageState("darkMode", {
    defaultValue: false,
  });
  const kolabpad = useRef<Kolabpad>();
  const id = useHash();

  const [readCodeConfirmOpen, setReadCodeConfirmOpen] = useState(false);
  const authErrorShownRef = useRef(false);
  const [otpFromServer, setOtpFromServer] = useState<string | null>(null);
  const [isAuthBlocked, setIsAuthBlocked] = useState(false);

  // Reset App-level state when navigating to a different document
  // Note: Sidebar state resets automatically via key={id} remounting
  useEffect(() => {
    setOtpFromServer(null);       // Clear OTP broadcasts from old document
    setIsAuthBlocked(false);       // Clear auth error state
    authErrorShownRef.current = false; // Allow new auth error to show
  }, [id]);

  useEffect(() => {
    if (editor?.getModel()) {
      // Comprehensively reset editor state to prevent leakage between documents
      resetEditorState(editor);

      kolabpad.current = new Kolabpad({
        uri: getWsUri(id),
        editor,
        onConnected: () => setConnection("connected"),
        onDisconnected: () => setConnection("disconnected"),
        onDesynchronized: () => {
          setConnection("desynchronized");
          toast({
            title: "Desynchronized with server",
            description: "Please save your work and refresh the page.",
            status: "error",
            duration: null,
          });
        },
        onAuthError: () => {
          if (!authErrorShownRef.current) {
            authErrorShownRef.current = true;
            setConnection("disconnected");
            setIsAuthBlocked(true);
          }
        },
        onChangeLanguage: (language) => {
          if (languages.includes(language)) {
            setLanguage(language);
          }
        },
        onChangeUsers: setUsers,
        onChangeOTP: (otp) => {
          setOtpFromServer(otp);
        },
      });
      return () => {
        kolabpad.current?.dispose();
        kolabpad.current = undefined;
      };
    }
  }, [id, editor, toast, setUsers]);

  useEffect(() => {
    if (connection === "connected") {
      kolabpad.current?.setInfo({ name, hue });
    }
  }, [connection, name, hue]);

  function handleLanguageChange(language: string) {
    setLanguage(language);
    if (kolabpad.current?.setLanguage(language)) {
      toast({
        title: "Language updated",
        description: (
          <>
            All users are now editing in{" "}
            <Text as="span" fontWeight="semibold">
              {language}
            </Text>
            .
          </>
        ),
        status: "info",
        duration: 2000,
        isClosable: true,
      });
    }
  }

  function handleLoadSample(confirmed: boolean) {
    if (editor?.getModel()) {
      const model = editor.getModel()!;
      const range = model.getFullModelRange();

      // If there are at least 10 lines of code, ask for confirmation.
      if (range.endLineNumber >= 10 && !confirmed) {
        setReadCodeConfirmOpen(true);
        return;
      }

      model.pushEditOperations(
        editor.getSelections(),
        [{ range, text: kolabpadRaw }],
        () => null,
      );
      editor.setPosition({ column: 0, lineNumber: 0 });
      if (language !== "go") {
        handleLanguageChange("go");
      }
    }
  }

  function handleDarkModeChange() {
    setDarkMode(!darkMode);
  }

  return (
    <Flex
      direction="column"
      h="100vh"
      overflow="hidden"
      bgColor={darkMode ? colors.dark.bg.primary : colors.light.bg.primary}
      color={darkMode ? colors.dark.text.primary : colors.light.text.primary}
    >
      <Box
        flexShrink={0}
        bgColor={darkMode ? colors.dark.bg.tertiary : colors.light.bg.tertiary}
        color={darkMode ? colors.dark.text.header : colors.light.text.secondary}
        textAlign="center"
        fontSize={layout.header.fontSize}
        py={layout.header.py}
      >
        Kolabpad
      </Box>
      <Flex flex="1 0" minH={0}>
        {!isAuthBlocked && (
          <Sidebar
            key={id} // Force complete remount when document changes
            documentId={id}
            connection={connection}
            darkMode={darkMode}
            language={language}
            currentUser={{ name, hue }}
            users={users}
            onDarkModeChange={handleDarkModeChange}
            onLanguageChange={handleLanguageChange}
            onLoadSample={() => handleLoadSample(false)}
            onChangeName={(name) => name.length > 0 && setName(name)}
            onChangeColor={() => setHue(generateHue())}
            otpFromServer={otpFromServer}
          />
        )}
        <AuthBlockedDialog isOpen={isAuthBlocked} />
        <ReadCodeConfirm
          isOpen={readCodeConfirmOpen}
          onClose={() => setReadCodeConfirmOpen(false)}
          onConfirm={() => {
            handleLoadSample(true);
            setReadCodeConfirmOpen(false);
          }}
        />

        <Flex flex={1} minW={0} h="100%" direction="column" overflow="hidden">
          <HStack
            h={layout.breadcrumb.height}
            spacing={1}
            color={darkMode ? colors.dark.text.muted : colors.light.text.muted}
            fontWeight="medium"
            fontSize={layout.breadcrumb.fontSize}
            px={layout.breadcrumb.px}
            flexShrink={0}
          >
            <Icon as={VscFolderOpened} fontSize="md" color={darkMode ? colors.dark.accent.folderIcon : colors.light.accent.folderIcon} />
            <Text>documents</Text>
            <Icon as={VscChevronRight} fontSize="md" />
            <Icon as={VscGist} fontSize="md" color={darkMode ? colors.dark.accent.documentIcon : colors.light.accent.documentIcon} />
            <Text>{id}</Text>
          </HStack>
          <Box flex={1} minH={0}>
            <Editor
              theme={darkMode ? "vs-dark" : "vs"}
              language={language}
              options={{
                automaticLayout: true,
                fontSize: 13,
              }}
              onMount={(editor) => setEditor(editor)}
            />
          </Box>
        </Flex>
      </Flex>
      <Footer />
    </Flex>
  );
}

export default App;
