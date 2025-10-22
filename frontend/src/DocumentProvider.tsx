import { createContext, useContext, ReactNode, useState, useRef, useEffect } from "react";
import { editor } from "monaco-editor/esm/vs/editor/editor.api";
import { useToast } from "@chakra-ui/react";
import Kolabpad, { UserInfo } from "./kolabpad";
import languages from "./languages.json";
import { useSession } from "./SessionProvider";
import { getOtpFromUrl } from "./utils/url";
import { logger } from "./logger";
import { USER } from "./constants";

/**
 * Document-scoped state that resets when switching documents.
 * The provider remounts when documentId changes (via key prop).
 */
interface OTPBroadcast {
  otp: string | null;
  userId: number;
  userName: string;
}

interface LanguageBroadcast {
  language: string;
  userId: number;
  userName: string;
}

interface DocumentContextValue {
  documentId: string;
  connection: "connected" | "disconnected" | "desynchronized";
  users: Record<number, UserInfo>;
  myUserId: number;
  language: string;
  sendLanguageChange: (language: string) => void;
  languageBroadcast: LanguageBroadcast | undefined;
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
  const [languageBroadcast, setLanguageBroadcast] = useState<LanguageBroadcast | undefined>(undefined);
  const [otpBroadcast, setOtpBroadcast] = useState<OTPBroadcast | undefined>(undefined);
  const [editor, setEditor] = useState<editor.IStandaloneCodeEditor>();
  const [isAuthBlocked, setIsAuthBlocked] = useState(false);

  const kolabpad = useRef<Kolabpad>();
  const authErrorShownRef = useRef(false);

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
      onChangeLanguage: (language, userId, userName) => {
        if (languages.includes(language)) {
          setLanguageBroadcast({ language, userId, userName });
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

  // Language broadcast handler - all clients update from broadcasts
  useEffect(() => {
    if (languageBroadcast === undefined) return;

    const isMyChange = languageBroadcast.userId === myUserId;
    const isInitialState = languageBroadcast.userId === USER.SYSTEM_USER_ID;

    // Update language state
    setLanguage(languageBroadcast.language);

    // Show appropriate toast (skip for initial state)
    if (isInitialState) {
      logger.debug('[Language] Initial state received, no toast');
    } else if (isMyChange) {
      toast({
        title: "Language updated",
        description: `All users are now editing in ${languageBroadcast.language}.`,
        status: "info",
        duration: 2000,
        isClosable: true,
      });
    } else {
      toast({
        title: "Language updated",
        description: (
          <>
            Language changed to {languageBroadcast.language} by <i>{languageBroadcast.userName}</i>
          </>
        ),
        status: "info",
        duration: 2000,
        isClosable: true,
      });
    }
  }, [languageBroadcast, myUserId, toast]);

  // Helper to send language change - just sends message, no local updates
  const sendLanguageChange = (newLanguage: string) => {
    kolabpad.current?.setLanguage(newLanguage);
  };

  return (
    <DocumentContext.Provider
      value={{
        documentId,
        connection,
        users,
        myUserId,
        language,
        sendLanguageChange,
        languageBroadcast,
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
