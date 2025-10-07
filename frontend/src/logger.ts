// Log levels: debug, info, error
type LogLevel = 'debug' | 'info' | 'error';

const logLevelOrder: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  error: 2,
};

// Get log level from build-time environment variable, default to 'info'
const getLogLevel = (): LogLevel => {
  const level = import.meta.env.VITE_LOG_LEVEL?.toLowerCase();
  if (level === 'debug' || level === 'info' || level === 'error') {
    return level;
  }
  return 'info';
};

const currentLevel = getLogLevel();
const currentLevelOrder = logLevelOrder[currentLevel];

// Log the current level on startup
console.log(`[LOGGER] Frontend log level: ${currentLevel}`);

// Logger functions
export const logger = {
  debug: (...args: any[]) => {
    if (currentLevelOrder <= logLevelOrder.debug) {
      console.log('[DEBUG]', ...args);
    }
  },

  info: (...args: any[]) => {
    if (currentLevelOrder <= logLevelOrder.info) {
      console.log('[INFO]', ...args);
    }
  },

  warn: (...args: any[]) => {
    if (currentLevelOrder <= logLevelOrder.info) {
      console.warn('[WARN]', ...args);
    }
  },

  error: (...args: any[]) => {
    console.error('[ERROR]', ...args);
  },
};
