import { useEffect, useState } from "react";

import { DOCUMENT } from "../constants";

/**
 * Extracts the document ID from the URL hash.
 * If no hash exists, generates a new random document ID.
 *
 * @returns The document ID (without query parameters)
 * @private
 */
function getHash(): string {
  const fullHash = window.location.hash.slice(1);

  if (!fullHash) {
    // No hash at all - generate new document ID
    let id = "";
    for (let i = 0; i < DOCUMENT.ID_LENGTH; i++) {
      id += DOCUMENT.ID_CHARS[Math.floor(Math.random() * DOCUMENT.ID_CHARS.length)];
    }
    window.history.replaceState(null, "", "#" + id);
    return id;
  }

  // Extract just the document ID (before any query parameters)
  // But DON'T modify the URL - preserve the full hash including OTP
  return fullHash.split('?')[0];
}

/**
 * Custom hook to manage document ID routing via URL hash.
 *
 * Automatically generates a new document ID if none exists in the URL.
 * Listens for hash changes and updates the document ID accordingly.
 * Preserves query parameters (like OTP) while extracting the document ID.
 *
 * @returns The current document ID from the URL hash
 *
 * @example
 * ```tsx
 * function App() {
 *   const documentId = useHash(); // "abc123" from "#abc123?otp=token"
 *   return <DocumentProvider documentId={documentId}>...</DocumentProvider>;
 * }
 * ```
 */
export function useHash(): string {
  const [hash, setHash] = useState(getHash);

  useEffect(() => {
    const handler = () => setHash(getHash());
    window.addEventListener("hashchange", handler);
    return () => window.removeEventListener("hashchange", handler);
  }, []);

  return hash;
}
