import type { ReactNode } from "react";

// Small line icons drawn in the text colour. The paths come from Lucide
// (https://lucide.dev, ISC licence: Copyright (c) Lucide Contributors).

function Icon({ children, size = 15 }: { children: ReactNode; size?: number }) {
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

// The pages of Settings and the layout switch, drawn on a 16 grid (GUI-041). One module
// holds every icon, so they share a style.
const PATHS: Record<string, string> = {
  back: "M10 3.5 5.5 8l4.5 4.5",
  "panes-1": "M2.5 3.5h11v9h-11z",
  "panes-2": "M2.5 3.5h11v9h-11zM8 3.5v9",
  "panes-3": "M2.5 3.5h11v9h-11zM8 3.5v9M8 8h5.5",
  "panes-4": "M2.5 3.5h11v9h-11zM8 3.5v9M2.5 8h11",
  providers: "M8 2.5 13 5.25v5.5L8 13.5 3 10.75v-5.5L8 2.5ZM3 5.25 8 8l5-2.75M8 8v5.5",
  profiles: "M3 4h10M3 8h10M3 12h6",
  skills: "M8 2.5l1.6 3.6 3.9.4-2.9 2.6.8 3.9L8 11l-3.4 2 .8-3.9-2.9-2.6 3.9-.4L8 2.5Z",
  mcp: "M6 2.5v3M10 2.5v3M4.5 5.5h7v2.5a3.5 3.5 0 0 1-7 0V5.5ZM8 11.5v2",
  optimizers: "M2.5 4h11l-4.2 4.6v4.1l-2.6-1.3V8.6L2.5 4Z",
  rules: "M8 2.5 13 4.5v3.5c0 3-2.2 5-5 6-2.8-1-5-3-5-6V4.5l5-2Z",
  environments: "M2.5 4.5a1.5 1.5 0 0 1 1.5-1.5h8a1.5 1.5 0 0 1 1.5 1.5v7a1.5 1.5 0 0 1-1.5 1.5H4a1.5 1.5 0 0 1-1.5-1.5v-7ZM5 6.5 7 8l-2 1.5M8.5 10H11",
  schedules: "M8 2.5a5.5 5.5 0 1 0 0 11 5.5 5.5 0 0 0 0-11ZM8 5v3l2 1.5",
  general:
    "M6.9 2.5h2.2l.3 1.6 1.2.7 1.5-.6 1.1 1.9-1.2 1v1.4l1.2 1-1.1 1.9-1.5-.6-1.2.7-.3 1.6H6.9l-.3-1.6-1.2-.7-1.5.6-1.1-1.9 1.2-1V7.1l-1.2-1 1.1-1.9 1.5.6 1.2-.7.3-1.6ZM8 6.2a1.8 1.8 0 1 0 0 3.6 1.8 1.8 0 0 0 0-3.6Z",
};

export function NavIcon({ name }: { name: string }) {
  return (
    <svg className="icon" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <path d={PATHS[name]} fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function line(...d: string[]) {
  return d.map((p, i) => <path key={i} d={p} />);
}

// Actions shown as an icon alone (GUI-041). Each button using one names its action in
// aria-label and title; the icon is never the name.
export const PanelRightIcon = () => <Icon><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M15 3v18" /></Icon>;
export const CompactIcon = () => <Icon>{line("M4 14h6v6", "M20 10h-6V4", "M14 10l7-7", "M3 21l7-7")}</Icon>;
export const ExpandIcon = () => <Icon>{line("M15 3h6v6", "M9 21H3v-6", "M21 3l-7 7", "M3 21l7-7")}</Icon>;
export const MoreIcon = () => (
  <Icon>
    <circle cx="12" cy="12" r="1" />
    <circle cx="19" cy="12" r="1" />
    <circle cx="5" cy="12" r="1" />
  </Icon>
);
export const NewSessionIcon = () => (
  <Icon>
    {line(
      "M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7",
      "M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z",
    )}
  </Icon>
);
export const BellIcon = () => (
  <Icon>
    {line("M10.268 21a2 2 0 0 0 3.464 0", "M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326")}
  </Icon>
);
export const BoardIcon = () => <Icon><rect x="3" y="3" width="18" height="18" rx="2" />{line("M9 3v18", "M15 3v18")}</Icon>;
export const UsageIcon = () => <Icon>{line("M3 3v16a2 2 0 0 0 2 2h16", "M18 17V9", "M13 17V5", "M8 17v-3")}</Icon>;
export const SendIcon = () => <Icon>{line("m5 12 7-7 7 7", "M12 19V5")}</Icon>;
export const QueueIcon = () => <Icon>{line("M11 12H3", "M16 6H3", "M16 18H3", "M18 9v6", "M21 12h-6")}</Icon>;
export const UpIcon = () => <Icon>{line("m18 15-6-6-6 6")}</Icon>;
export const DownIcon = () => <Icon>{line("m6 9 6 6 6-6")}</Icon>;
export const AllowedIcon = () => <Icon>{line("M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z", "m9 12 2 2 4-4")}</Icon>;
export const RuleIcon = () => <Icon>{line("M15 12h-5", "M15 8h-5", "M19 17V5a2 2 0 0 0-2-2H4", "M8 21h12a2 2 0 0 0 2-2v-1a1 1 0 0 0-1-1H11a1 1 0 0 0-1 1v1a2 2 0 1 1-4 0V5a2 2 0 1 0-4 0v2a1 1 0 0 0 1 1h3")}</Icon>;
export const InfoIcon = () => (
  <Icon>
    <circle cx="12" cy="12" r="10" />
    {line("M12 16v-4", "M12 8h.01")}
  </Icon>
);

// A git branch, from Lucide's git-branch: marks a session in a repository.
export const GitBranchIcon = () => (
  <Icon size={12}>
    {line("M6 3v12", "M18 9a9 9 0 0 1-9 9")}
    <circle cx="18" cy="6" r="3" />
    <circle cx="6" cy="18" r="3" />
  </Icon>
);

