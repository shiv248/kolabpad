/**
 * Broadcast state types for synchronized features (OTP, Language)
 */

/** OTP protection broadcast state */
export interface OTPBroadcast {
  otp: string | null;
  userId: number;
  userName: string;
}

/** Language change broadcast state */
export interface LanguageBroadcast {
  language: string;
  userId: number;
  userName: string;
}
