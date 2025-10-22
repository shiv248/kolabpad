import {
  AlertDialog,
  AlertDialogBody,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogOverlay,
  Button,
} from "@chakra-ui/react";
import { useRef } from "react";

export type AuthBlockedDialogProps = {
  isOpen: boolean;
};

/** Non-dismissible dialog shown when user tries to access OTP-protected document without valid OTP. */
function AuthBlockedDialog({ isOpen }: AuthBlockedDialogProps) {
  const buttonRef = useRef<HTMLButtonElement>(null);

  const handleGotIt = () => {
    window.location.href = "/";
  };

  return (
    <AlertDialog
      isOpen={isOpen}
      leastDestructiveRef={buttonRef}
      onClose={() => {}} // No-op: dialog is non-dismissible
      closeOnOverlayClick={false}
      closeOnEsc={false}
    >
      <AlertDialogOverlay>
        <AlertDialogContent>
          <AlertDialogHeader>Authentication Required</AlertDialogHeader>

          <AlertDialogBody>
            This document is password-protected. You need the correct link with
            the security token to access it.
          </AlertDialogBody>

          <AlertDialogFooter>
            <Button ref={buttonRef} colorScheme="blue" onClick={handleGotIt}>
              Got it!
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialogOverlay>
    </AlertDialog>
  );
}

export default AuthBlockedDialog;
