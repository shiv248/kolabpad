import { createContext, useContext, ReactNode, useState, useRef, useEffect } from "react";
import { editor } from "monaco-editor/esm/vs/editor/editor.api";
import { useToast } from "@chakra-ui/react";
import Kolabpad, { UserInfo } from "./kolabpad";
import languages from "./languages.json";
import { useSession } from "./SessionProvider";
import { getOtpFromUrl } from "./utils/url";
import { logger } from "./logger";

/**
 * Document-scoped state that resets when switching documents.
 * The provider remounts when documentId changes (via key prop).
 */
interface OTPBroadcast {
  otp: string | null;
  userId: number;
  userName: string;
}

interface DocumentContextValue {
  documentId: string;
  connection: "connected" | "disconnected" | "desynchronized";
  users: Record<number, UserInfo>;
  myUserId: number;
  language: string;
  setLanguage: (language: string) => void;
  otpBroadcast: OTPBroadcast | undefined;
  editor: editor.IStandaloneCodeEditor | undefined;
  setEditor: (editor: editor.IStandaloneCodeEditor) => void;
  isAuthBlocked: boolean;
  setIsAuthBlocked: (blocked: boolean) => void;
}

const DocumentContext = createContext<DocumentContextValue | undefined>(undefined);

function getWsUri(id: string) {
  let url = new URL(`api/socket/${id}`, window.location.href);
  url.protocol = url.protocol == "https:" ? "wss:" : "ws:";

  // Add OTP parameter if present in the URL
  const otp = getOtpFromUrl();
  if (otp) {
    url.searchParams.set('otp', otp);
  }

  return url.href;
}

export function DocumentProvider({
  documentId,
  children
}: {
  documentId: string;
  children: ReactNode;
}) {
  const toast = useToast();
  const { name, hue } = useSession();

  // Document-scoped state
  const [connection, setConnection] = useState<"connected" | "disconnected" | "desynchronized">("disconnected");
  const [users, setUsers] = useState<Record<number, UserInfo>>({});
  const [myUserId, setMyUserId] = useState<number>(-1);
  const [language, setLanguage] = useState("plaintext");
  const [otpBroadcast, setOtpBroadcast] = useState<OTPBroadcast | undefined>(undefined);
  const [editor, setEditor] = useState<editor.IStandaloneCodeEditor>();
  const [isAuthBlocked, setIsAuthBlocked] = useState(false);

  const kolabpad = useRef<Kolabpad>();
  const authErrorShownRef = useRef(false);
  const ignoreLangBroadcastRef = useRef(false); // Track if we initiated the language change

  // Initialize Kolabpad instance when editor is ready
  useEffect(() => {
    if (!editor?.getModel()) return;

    kolabpad.current = new Kolabpad({
      uri: getWsUri(documentId),
      editor,
      onConnected: () => setConnection("connected"),
      onDisconnected: () => setConnection("disconnected"),
      onIdentity: (userId) => {
        setMyUserId(userId);
        logger.debug('[Identity] User ID set:', userId);
      },
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
          // If we initiated this change, ignore the broadcast (we already showed our toast)
          if (ignoreLangBroadcastRef.current) {
            ignoreLangBroadcastRef.current = false;
            setLanguage(language);
            return;
          }

          // Another user changed the language - show notification
          setLanguage(language);
          toast({
            title: "Language updated",
            description: `Another user changed the language to ${language}.`,
            status: "info",
            duration: 2000,
            isClosable: true,
          });
        }
      },
      onChangeUsers: setUsers,
      onChangeOTP: (otp, userId, userName) => {
        setOtpBroadcast({ otp, userId, userName });
      },
    });

    return () => {
      kolabpad.current?.dispose();
      kolabpad.current = undefined;
    };
  }, [documentId, editor, toast]);

  // Update user info when connected
  useEffect(() => {
    if (connection === "connected") {
      kolabpad.current?.setInfo({ name, hue });
    }
  }, [connection, name, hue]);

  // Helper to set language and broadcast to other users
  const handleSetLanguage = (newLanguage: string) => {
    setLanguage(newLanguage);
    if (kolabpad.current?.setLanguage(newLanguage)) {
      // Mark that we initiated this change to ignore the broadcast echo
      ignoreLangBroadcastRef.current = true;

      toast({
        title: "Language updated",
        description: `All users are now editing in ${newLanguage}.`,
        status: "info",
        duration: 2000,
        isClosable: true,
      });
    }
  };

  return (
    <DocumentContext.Provider
      value={{
        documentId,
        connection,
        users,
        myUserId,
        language,
        setLanguage: handleSetLanguage,
        otpBroadcast,
        editor,
        setEditor,
        isAuthBlocked,
        setIsAuthBlocked,
      }}
    >
      {children}
    </DocumentContext.Provider>
  );
}

/**
 * Hook to access document-scoped state.
 * Must be used within a DocumentProvider.
 */
export function useDocument() {
  const context = useContext(DocumentContext);
  if (context === undefined) {
    throw new Error("useDocument must be used within a DocumentProvider");
  }
  return context;
}
