import { useEffect, useState } from "react";
import { describeActivity, type Activity } from "./sessionActivity.ts";

// ActivityLine shows what a session is doing (GUI-030), and keeps its time current.
export default function ActivityLine({ status, activity, roots }: { status: string; activity?: Activity; roots: string[] }) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 15000);
    return () => clearInterval(timer);
  }, []);
  const text = describeActivity(status, activity, roots, now);
  return text ? (
    <span className="muted session-activity" title={text}>
      {text}
    </span>
  ) : null;
}
