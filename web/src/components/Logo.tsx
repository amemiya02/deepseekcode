// DeepSeekCode brand mark — the "two arrows" double chevron (spec §3.1).
// Front chevron solid #4d6bfe; back chevron same blue at 50% opacity.
export function Logo({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="27 30 44 40"
      fill="none"
      role="img"
      aria-label="DeepSeekCode"
      className={className}
    >
      <path
        d="M33 36 L49 50 L33 64"
        stroke="#4d6bfe"
        strokeWidth="9"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M47 36 L63 50 L47 64"
        stroke="#4d6bfe"
        strokeWidth="9"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity="0.5"
      />
    </svg>
  )
}
