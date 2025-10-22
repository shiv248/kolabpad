/**
 * Tests for API client
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiFetch, ApiError } from './client';

// Mock global fetch
const mockFetch = vi.fn();
global.fetch = mockFetch as any;

describe('apiFetch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('should make successful GET request', async () => {
    const mockData = { success: true };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      statusText: 'OK',
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => mockData,
    });

    const result = await apiFetch('/test');

    expect(mockFetch).toHaveBeenCalledWith(
      '/test',
      expect.objectContaining({
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );
    expect(result).toEqual(mockData);
  });

  it('should make successful POST request with body', async () => {
    const requestBody = { name: 'test' };
    const mockData = { id: 1 };

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 201,
      statusText: 'Created',
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => mockData,
    });

    const result = await apiFetch('/test', {
      method: 'POST',
      body: requestBody,
    });

    expect(mockFetch).toHaveBeenCalledWith(
      '/test',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify(requestBody),
      })
    );
    expect(result).toEqual(mockData);
  });

  it('should throw ApiError on 4xx error', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      text: async () => 'Resource not found',
    });

    await expect(apiFetch('/test')).rejects.toThrow(ApiError);

    try {
      await apiFetch('/test');
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect((error as ApiError).status).toBe(404);
      expect((error as ApiError).statusText).toBe('Not Found');
    }
  });

  it('should throw ApiError on 5xx error', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      text: async () => 'Server error occurred',
    });

    await expect(apiFetch('/test')).rejects.toThrow(ApiError);
  });

  it('should handle 204 No Content response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 204,
      statusText: 'No Content',
      headers: new Headers(),
    });

    const result = await apiFetch('/test', { method: 'DELETE' });

    expect(result).toEqual({});
  });

  it('should handle network errors', async () => {
    mockFetch.mockRejectedValue(new Error('Network failure'));

    await expect(apiFetch('/test')).rejects.toThrow(ApiError);

    try {
      await apiFetch('/test');
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect((error as ApiError).message).toContain('Network failure');
    }
  });

  it('should handle non-JSON responses', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      statusText: 'OK',
      headers: new Headers({ 'content-type': 'text/plain' }),
      text: async () => 'Plain text response',
    });

    const result = await apiFetch('/test');

    expect(result).toBe('Plain text response');
  });

  it('should merge custom headers', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({}),
    });

    await apiFetch('/test', {
      headers: {
        'Authorization': 'Bearer token',
      },
    });

    expect(mockFetch).toHaveBeenCalledWith(
      '/test',
      expect.objectContaining({
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer token',
        },
      })
    );
  });

  it('should handle error when response text fails', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      text: async () => {
        throw new Error('Cannot read response');
      },
    });

    try {
      await apiFetch('/test');
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect((error as ApiError).message).toContain('Unknown error');
    }
  });
});
