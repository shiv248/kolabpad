import { createContext, useContext, ReactNode, useState, useRef, useEffect } from "react";
import { editor } from "monaco-editor/esm/vs/editor/editor.api";
import { useToast } from "@chakra-ui/react";
import Kolabpad from "../services/kolabpad";
import languages from "../languages.json";
import { useSession } from "./SessionProvider";
import { getOtpFromUrl } from "../utils/url";
import { logger } from "../logger";
import { useLanguageSync } from "../hooks/useLanguageSync";
import { useColorCollision } from "../hooks/useColorCollision";
import type { UserInfo, OTPBroadcast, LanguageBroadcast } from "../types";

/**
 * Document-scoped state that resets when switching documents.
 * The provider remounts when documentId changes (via key prop).
 */

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
  const { name, hue, setHue } = useSession();

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
    if (!editor?.getModel()) {
      logger.debug('[DocumentProvider] Waiting for editor model');
      return;
    }

    logger.info('[DocumentProvider] Initializing Kolabpad for document:', documentId);

    kolabpad.current = new Kolabpad({
      uri: getWsUri(documentId),
      editor,
      onConnected: () => {
        logger.info('[DocumentProvider] Connected to document:', documentId);
        setConnection("connected");
      },
      onDisconnected: () => {
        logger.info('[DocumentProvider] Disconnected from document:', documentId);
        setConnection("disconnected");
      },
      onIdentity: (userId) => {
        setMyUserId(userId);
        logger.info('[DocumentProvider] User ID assigned:', userId);
      },
      onDesynchronized: () => {
        logger.error('[DocumentProvider] Desynchronized from server');
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
          logger.error('[DocumentProvider] Authentication failed');
          authErrorShownRef.current = true;
          setConnection("disconnected");
          setIsAuthBlocked(true);
        }
      },
      onChangeLanguage: (language, userId, userName) => {
        logger.debug('[DocumentProvider] Language broadcast received:', { language, userId, userName });
        if (languages.includes(language)) {
          setLanguageBroadcast({ language, userId, userName });
        } else {
          logger.error('[DocumentProvider] Invalid language received:', language);
        }
      },
      onChangeUsers: (users) => {
        logger.debug('[DocumentProvider] Users updated, count:', Object.keys(users).length);
        setUsers(users);
      },
      onChangeOTP: (otp, userId, userName) => {
        logger.debug('[DocumentProvider] OTP broadcast received:', { otp: otp ? '[REDACTED]' : null, userId, userName });
        setOtpBroadcast({ otp, userId, userName });
      },
    });

    return () => {
      logger.info('[DocumentProvider] Cleaning up Kolabpad instance');
      kolabpad.current?.dispose();
      kolabpad.current = undefined;
    };
  }, [documentId, editor, toast]);

  // Update user info when connected
  useEffect(() => {
    if (connection === "connected") {
      logger.debug('[DocumentProvider] Updating user info:', { name, hue });
      kolabpad.current?.setInfo({ name, hue });
    }
  }, [connection, name, hue]);

  // Use custom hooks for broadcast handling and collision detection
  useLanguageSync({
    languageBroadcast,
    myUserId,
    onLanguageChange: setLanguage,
  });

  useColorCollision({
    connection,
    myUserId,
    users,
    currentHue: hue,
    onHueChange: setHue,
  });

  // Helper to send language change - just sends message, no local updates
  const sendLanguageChange = (newLanguage: string) => {
    logger.debug('[DocumentProvider] Sending language change:', newLanguage);
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
