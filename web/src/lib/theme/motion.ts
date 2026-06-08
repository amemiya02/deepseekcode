// Motion contract aligned to DeepSeek-GUI DESIGN.md §6 (timings in ms).
// Functional, not decorative: confirm clicks, reveal hover, smooth panels,
// indicate liveness. Brand-neutral — no color here.
export const MOTION = {
  micro: 140,      // hover bg / border / focus ring swap
  standard: 150,   // card hover lift, composer border on focus
  deep: 300,       // modal open, route transition
  pulse: 1800,     // looped status dot / work logo
  shiny: 2400,     // looped streaming-text shimmer
  cardLift: 'translateY(-1px)',
  press: 'scale(0.985)',
  easing: 'cubic-bezier(0.2, 0, 0, 1)',
} as const
