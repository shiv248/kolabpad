import { useEffect, useState } from "react";

const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
const idLen = 6;

function getHash() {
  const fullHash = window.location.hash.slice(1);

  if (!fullHash) {
    // No hash at all - generate new document ID
    let id = "";
    for (let i = 0; i < idLen; i++) {
      id += chars[Math.floor(Math.random() * chars.length)];
    }
    window.history.replaceState(null, "", "#" + id);
    return id;
  }

  // Extract just the document ID (before any query parameters)
  // But DON'T modify the URL - preserve the full hash including OTP
  return fullHash.split('?')[0];
}

function useHash() {
  const [hash, setHash] = useState(getHash);

  useEffect(() => {
    const handler = () => setHash(getHash());
    window.addEventListener("hashchange", handler);
    return () => window.removeEventListener("hashchange", handler);
  }, []);

  return hash;
}

export default useHash;
