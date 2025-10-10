/**
 * Extract the OTP parameter from the URL hash.
 *
 * URL format: #documentId?otp=token
 *
 * @returns The OTP token if present in the URL, null otherwise
 */
export function getOtpFromUrl(): string | null {
  const hashParts = window.location.hash.slice(1).split('?');
  const params = new URLSearchParams(hashParts[1] || '');
  return params.get('otp');
}
