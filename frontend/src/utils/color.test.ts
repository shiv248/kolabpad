/**
 * Tests for color utility functions
 */

import { describe, it, expect } from 'vitest';
import { generateHue, hasHueCollision } from './color';

describe('generateHue', () => {
  it('should generate a hue between 0 and 360', () => {
    const hue = generateHue();
    expect(hue).toBeGreaterThanOrEqual(0);
    expect(hue).toBeLessThan(360);
  });

  it('should avoid collisions with existing hues', () => {
    const existingHues = [10, 50, 90, 130, 170];
    const newHue = generateHue(existingHues);

    // New hue should not collide with any existing hue
    existingHues.forEach((existingHue) => {
      expect(hasHueCollision(newHue, [existingHue])).toBe(false);
    });
  });

  it('should return a hue even when all hues are taken (after 20 attempts)', () => {
    // Fill all hue space (collision threshold is 15 degrees, so ~24 hues max)
    const densePacking: number[] = [];
    for (let i = 0; i < 360; i += 15) {
      densePacking.push(i);
    }

    const hue = generateHue(densePacking);

    // Should still return a valid hue (0-360) even if it collides
    expect(hue).toBeGreaterThanOrEqual(0);
    expect(hue).toBeLessThan(360);
  });

  it('should generate different hues on multiple calls', () => {
    const hues = new Set<number>();
    for (let i = 0; i < 10; i++) {
      hues.add(generateHue());
    }

    // With high probability, at least some should be different
    // (could theoretically fail with very bad RNG, but extremely unlikely)
    expect(hues.size).toBeGreaterThan(1);
  });
});

describe('hasHueCollision', () => {
  it('should detect collision when hues are within threshold', () => {
    const hue = 100;
    const existingHues = [105]; // Within 15-degree threshold

    expect(hasHueCollision(hue, existingHues)).toBe(true);
  });

  it('should not detect collision when hues are far apart', () => {
    const hue = 100;
    const existingHues = [200]; // 100 degrees apart

    expect(hasHueCollision(hue, existingHues)).toBe(false);
  });

  it('should handle wraparound at 360 degrees', () => {
    const hue = 5;
    const existingHues = [355]; // 10 degrees apart with wraparound

    expect(hasHueCollision(hue, existingHues)).toBe(true);
  });

  it('should handle reverse wraparound', () => {
    const hue = 355;
    const existingHues = [5]; // 10 degrees apart with wraparound

    expect(hasHueCollision(hue, existingHues)).toBe(true);
  });

  it('should return false when no existing hues', () => {
    const hue = 100;
    const existingHues: number[] = [];

    expect(hasHueCollision(hue, existingHues)).toBe(false);
  });

  it('should detect collision with any of multiple existing hues', () => {
    const hue = 100;
    const existingHues = [50, 105, 200]; // Middle one collides

    expect(hasHueCollision(hue, existingHues)).toBe(true);
  });

  it('should not detect collision when outside threshold for all hues', () => {
    const hue = 100;
    const existingHues = [50, 150, 200]; // All > 15 degrees away

    expect(hasHueCollision(hue, existingHues)).toBe(false);
  });
});
