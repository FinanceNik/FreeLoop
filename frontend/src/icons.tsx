import type { ReactElement } from "react";

// Small line icons drawn in the text colour. The paths come from Lucide
// (https://lucide.dev, ISC licence: Copyright (c) Lucide Contributors).

function Icon({ children, size = 15 }: { children: ReactElement | ReactElement[]; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export function CopyIcon() {
  return (
    <Icon>
      <rect x="9" y="9" width="13" height="13" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </Icon>
  );
}

export function CheckIcon() {
  return (
    <Icon>
      <path d="M20 6 9 17l-5-5" />
    </Icon>
  );
}

export function RewindIcon() {
  return (
    <Icon>
      <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
      <path d="M3 3v5h5" />
    </Icon>
  );
}

// LayoutIcon draws a workspace layout of n panes (GUI-017): one, two side
// by side, one large and two stacked, or four in a grid.
export function LayoutIcon({ n }: { n: 1 | 2 | 3 | 4 }) {
  return (
    <Icon>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      {n > 1 ? <line x1="12" y1="4" x2="12" y2="20" /> : <></>}
      {n === 3 ? <line x1="12" y1="12" x2="21" y2="12" /> : <></>}
      {n === 4 ? <line x1="3" y1="12" x2="21" y2="12" /> : <></>}
    </Icon>
  );
}

// The mark, from the author's website: three circles whose colours mix where they meet.
export function Logo({ size = 18 }: { size?: number }) {
  return (
    <svg className="logo" width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" focusable="false">
      <circle cx="32" cy="23.38" r="16.5" fill="#6dcade" fillOpacity="0.7" />
      <circle cx="22.04" cy="40.62" r="16.5" fill="#7da0ee" fillOpacity="0.7" />
      <circle cx="41.96" cy="40.62" r="16.5" fill="#f3dc80" fillOpacity="0.7" />
    </svg>
  );
}

export function FilterIcon() {
  return (
    <Icon>
      <path d="M10 20a1 1 0 0 0 .553.895l2 1A1 1 0 0 0 14 21v-7a2 2 0 0 1 .517-1.341L21.74 4.67A1 1 0 0 0 21 3H3a1 1 0 0 0-.742 1.67l7.225 7.989A2 2 0 0 1 10 14z" />
    </Icon>
  );
}

export function ChevronIcon() {
  return (
    <Icon size={12}>
      <path d="m9 18 6-6-6-6" />
    </Icon>
  );
}

export function PaperclipIcon() {
  return (
    <Icon>
      <path d="m16 6-8.414 8.586a2 2 0 0 0 2.829 2.829l8.414-8.586a4 4 0 1 0-5.657-5.657l-8.379 8.551a6 6 0 1 0 8.485 8.485l8.379-8.551" />
    </Icon>
  );
}

export function CloseIcon() {
  return (
    <Icon size={14}>
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </Icon>
  );
}

export function ForkIcon() {
  return (
    <Icon>
      <line x1="6" y1="3" x2="6" y2="15" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 0 1-9 9" />
    </Icon>
  );
}

export function StopIcon() {
  return (
    <Icon size={13}>
      <rect x="6" y="6" width="12" height="12" rx="2" />
    </Icon>
  );
}
