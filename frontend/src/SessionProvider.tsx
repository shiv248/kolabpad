import { createContext, useContext, ReactNode } from "react";
import useLocalStorageState from "use-local-storage-state";
import animals from "./animals.json";

function generateName() {
  return "Anonymous " + animals[Math.floor(Math.random() * animals.length)];
}

function generateHue() {
  return Math.floor(Math.random() * 360);
}

/**
 * Session-scoped state that persists across document changes.
 * Includes user preferences like name, hue, and dark mode.
 */
interface SessionContextValue {
  name: string;
  setName: (name: string) => void;
  hue: number;
  setHue: (hue: number) => void;
  darkMode: boolean;
  setDarkMode: (darkMode: boolean) => void;
}

const SessionContext = createContext<SessionContextValue | undefined>(undefined);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [name, setName] = useLocalStorageState("name", {
    defaultValue: generateName,
  });
  const [hue, setHue] = useLocalStorageState("hue", {
    defaultValue: generateHue,
  });
  const [darkMode, setDarkMode] = useLocalStorageState("darkMode", {
    defaultValue: false,
  });

  return (
    <SessionContext.Provider
      value={{
        name,
        setName,
        hue,
        setHue,
        darkMode,
        setDarkMode,
      }}
    >
      {children}
    </SessionContext.Provider>
  );
}

/**
 * Hook to access session-scoped state (user preferences).
 * Must be used within a SessionProvider.
 */
export function useSession() {
  const context = useContext(SessionContext);
  if (context === undefined) {
    throw new Error("useSession must be used within a SessionProvider");
  }
  return context;
}
