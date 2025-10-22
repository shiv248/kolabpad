/**
 * Vitest test setup
 * Configures test environment, mocks, and custom matchers
 */

import '@testing-library/jest-dom';
import { expect, afterEach, vi } from 'vitest';
import { cleanup } from '@testing-library/react';

// Cleanup after each test
afterEach(() => {
  cleanup();
});

// Mock window.location
Object.defineProperty(window, 'location', {
  writable: true,
  value: {
    hash: '',
    href: 'http://localhost:3000',
    origin: 'http://localhost:3000',
    pathname: '/',
    search: '',
    host: 'localhost:3000',
    hostname: 'localhost',
    port: '3000',
    protocol: 'http:',
    assign: vi.fn(),
    reload: vi.fn(),
    replace: vi.fn(),
  },
});

// Mock navigator.clipboard
Object.defineProperty(navigator, 'clipboard', {
  writable: true,
  value: {
    writeText: vi.fn().mockResolvedValue(undefined),
    readText: vi.fn().mockResolvedValue(''),
  },
});

// Mock window.history
Object.defineProperty(window, 'history', {
  writable: true,
  value: {
    pushState: vi.fn(),
    replaceState: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    go: vi.fn(),
    length: 0,
    scrollRestoration: 'auto' as ScrollRestoration,
    state: null,
  },
});
