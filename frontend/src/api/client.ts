/**
 * Base HTTP client with error handling and common request configuration
 */

import { logger } from "../logger";

/** Generic API error */
export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status?: number,
    public readonly statusText?: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Base fetch options */
export interface FetchOptions extends Omit<RequestInit, 'body'> {
  body?: any;
}

/**
 * Enhanced fetch wrapper with:
 * - JSON serialization
 * - Error handling
 * - Logging
 */
export async function apiFetch<T = any>(
  url: string,
  options: FetchOptions = {}
): Promise<T> {
  const { body, headers = {}, ...restOptions } = options;

  // Serialize body if present
  const requestInit: RequestInit = {
    ...restOptions,
    headers: {
      'Content-Type': 'application/json',
      ...headers,
    },
  };

  if (body !== undefined) {
    requestInit.body = JSON.stringify(body);
  }

  logger.debug(`[API] ${options.method || 'GET'} ${url}`, body);

  try {
    const response = await fetch(url, requestInit);

    if (!response.ok) {
      const errorText = await response.text().catch(() => 'Unknown error');
      logger.error(`[API] ${response.status} ${response.statusText}:`, errorText);
      throw new ApiError(
        errorText || `Request failed: ${response.statusText}`,
        response.status,
        response.statusText
      );
    }

    // Try to parse JSON response, fallback to text
    const contentType = response.headers.get('content-type');
    if (contentType?.includes('application/json')) {
      return await response.json();
    }

    // If no content, return empty object
    if (response.status === 204) {
      return {} as T;
    }

    return await response.text() as any;
  } catch (error) {
    if (error instanceof ApiError) {
      throw error;
    }
    logger.error('[API] Request failed:', error);
    throw new ApiError(
      error instanceof Error ? error.message : 'Network request failed'
    );
  }
}
