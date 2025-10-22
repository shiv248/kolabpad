/**
 * Color utility functions for user identification.
 */

/**
 * Generates a random hue value, optionally avoiding collisions with existing hues.
 *
 * @param existingHues - Array of hues already in use by other users
 * @returns A hue value between 0-359
 */
export function generateHue(existingHues: number[] = []): number {
  const MIN_DIFFERENCE = 30; // Minimum degrees between colors on the color wheel
  const MAX_ATTEMPTS = 20; // Maximum attempts to find a distinct color

  let hue;
  let attempts = 0;

  do {
    hue = Math.floor(Math.random() * 360);
    attempts++;

    // Give up if we can't find a distinct color after MAX_ATTEMPTS
    if (attempts >= MAX_ATTEMPTS) break;
  } while (
    existingHues.some(existing => {
      const diff = Math.abs(existing - hue);
      // Check both direct difference and wrap-around difference (e.g., 350 vs 10)
      return diff < MIN_DIFFERENCE || diff > (360 - MIN_DIFFERENCE);
    })
  );

  return hue;
}

/**
 * Checks if a hue is too similar to any existing hues.
 *
 * @param hue - The hue to check
 * @param existingHues - Array of hues already in use
 * @returns True if there's a collision, false otherwise
 */
export function hasHueCollision(hue: number, existingHues: number[]): boolean {
  const MIN_DIFFERENCE = 30;

  return existingHues.some(existing => {
    const diff = Math.abs(existing - hue);
    return diff < MIN_DIFFERENCE || diff > (360 - MIN_DIFFERENCE);
  });
}
