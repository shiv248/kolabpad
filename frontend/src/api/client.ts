/**
 * Base HTTP client with error handling and common request configuration
 */

import { logger } from "../logger";

/**
 * API error class with HTTP status information
 */
export class ApiError extends Error {
  /**
   * Creates an API error
   * @param message - Error message
   * @param status - HTTP status code (e.g., 404, 500)
   * @param statusText - HTTP status text (e.g., "Not Found")
   */
  constructor(
    message: string,
    public readonly status?: number,
    public readonly statusText?: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Base fetch options with typed body */
export interface FetchOptions extends Omit<RequestInit, 'body'> {
  /** Request body (will be JSON stringified) */
  body?: any;
}

/**
 * Enhanced fetch wrapper with error handling and logging.
 *
 * Features:
 * - Automatic JSON serialization of request body
 * - Automatic JSON parsing of response
 * - HTTP error handling with ApiError
 * - Request/response logging
 * - Support for 204 No Content
 *
 * @param url - API endpoint URL
 * @param options - Fetch options (body will be JSON stringified)
 * @returns Parsed response data
 *
 * @throws {ApiError} When HTTP request fails (4xx, 5xx) or network error occurs
 *
 * @example
 * ```ts
 * // GET request
 * const data = await apiFetch('/api/users');
 *
 * // POST request
 * const user = await apiFetch('/api/users', {
 *   method: 'POST',
 *   body: { name: 'Alice' }
 * });
 * ```
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

    // Log successful responses for mutating operations
    const method = options.method || 'GET';
    if (method !== 'GET') {
      logger.info(`[API] ${method} ${url} succeeded (${response.status})`);
    }

    // Try to parse JSON response, fallback to text
    const contentType = response.headers.get('content-type');
    if (contentType?.includes('application/json')) {
      const data = await response.json();
      logger.debug(`[API] Response data:`, data);
      return data;
    }

    // If no content, return empty object
    if (response.status === 204) {
      logger.debug(`[API] No content (204)`);
      return {} as T;
    }

    const textData = await response.text();
    logger.debug(`[API] Text response:`, textData);
    return textData as any;
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
