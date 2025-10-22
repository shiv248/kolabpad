import { Box, Flex, HStack, Icon, Text } from "@chakra-ui/react";
import Editor from "@monaco-editor/react";
import { editor } from "monaco-editor/esm/vs/editor/editor.api";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api";
import { useState, useEffect } from "react";
import { VscChevronRight, VscFolderOpened, VscGist } from "react-icons/vsc";

import kolabpadRaw from "../../pkg/server/kolabpad.go?raw";
import AuthBlockedDialog from "./AuthBlockedDialog";
import Footer from "./Footer";
import ReadCodeConfirm from "./ReadCodeConfirm";
import Sidebar from "./Sidebar";
import { colors, layout } from "./theme";
import useHash from "./useHash";
import { SessionProvider, useSession } from "./SessionProvider";
import { DocumentProvider, useDocument } from "./DocumentProvider";
import { generateHue } from "./utils/color";

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

/**
 * Inner component that has access to both Session and Document contexts.
 * This allows us to use the hooks to access state from providers.
 */
function DocumentEditor() {
  const { name, hue, setName, setHue, darkMode, setDarkMode } = useSession();
  const {
    documentId,
    connection,
    users,
    myUserId,
    language,
    sendLanguageChange,
    otpBroadcast,
    editor,
    setEditor,
    isAuthBlocked,
  } = useDocument();

  const [readCodeConfirmOpen, setReadCodeConfirmOpen] = useState(false);

  // Reset editor state when document changes or editor mounts
  useEffect(() => {
    if (editor?.getModel()) {
      resetEditorState(editor);
    }
  }, [editor, documentId]);

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
        sendLanguageChange("go");
      }
    }
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
            documentId={documentId}
            connection={connection}
            darkMode={darkMode}
            language={language}
            currentUser={{ id: myUserId, name, hue }}
            users={users}
            onDarkModeChange={() => setDarkMode(!darkMode)}
            onLanguageChange={sendLanguageChange}
            onLoadSample={() => handleLoadSample(false)}
            onChangeName={(name) => name.length > 0 && setName(name)}
            onChangeColor={() => setHue(generateHue(Object.values(users).map(u => u.hue)))}
            otpBroadcast={otpBroadcast}
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
            <Text>{documentId}</Text>
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

/**
 * Main App component with provider hierarchy.
 * SessionProvider wraps DocumentProvider to provide session-scoped state.
 * DocumentProvider remounts when documentId changes (via key prop).
 */
function App() {
  const documentId = useHash();

  return (
    <SessionProvider>
      <DocumentProvider key={documentId} documentId={documentId}>
        <DocumentEditor />
      </DocumentProvider>
    </SessionProvider>
  );
}

export default App;
