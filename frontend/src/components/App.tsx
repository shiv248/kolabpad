/**
 * Main App component
 * Sets up the provider hierarchy and routing
 */

import { useHash } from "../hooks";
import { SessionProvider } from "../contexts/SessionProvider";
import { DocumentProvider } from "../contexts/DocumentProvider";
import DocumentEditor from "./document/DocumentEditor";

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
