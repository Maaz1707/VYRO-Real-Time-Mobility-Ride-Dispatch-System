"use client";
import { useEffect, useState } from "react";
import { Activity } from "lucide-react";

type Pulse = {
  storage: string;
  uptimeSeconds: number;
  httpRequests: number;
  httpServerErrors: number;
  sampleCount: number;
  recentP95Ms: number;
  dispatchErrors: number;
};

export default function OperationsPanel() {
  const [pulse, setPulse] = useState<Pulse | null>(null);
  const [available, setAvailable] = useState(false);
  useEffect(() => {
    let mounted = true;
    async function refresh() {
      try {
        const response = await fetch("/api/ops", { cache: "no-store" });
        if (!response.ok) throw Error("unavailable");
        const result = await response.json();
        if (mounted) {
          setPulse(result);
          setAvailable(true);
        }
      } catch {
        if (mounted) setAvailable(false);
      }
    }
    refresh();
    const timer = setInterval(refresh, 5000);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, []);

  return (
    <section className="operations-panel" aria-label="Service pulse">
      <div className="pulse-title">
        <Activity size={18} />
        <strong>Service pulse</strong>
        <span>
          {available
            ? pulse?.storage === "postgres"
              ? "PostgreSQL"
              : "Memory mode"
            : "Reconnecting…"}
        </span>
      </div>
      {pulse && available && (
        <div className="pulse-values">
          <span>
            Uptime{" "}
            <b>
              {Math.floor(pulse.uptimeSeconds / 60)}m{" "}
              {Math.floor(pulse.uptimeSeconds % 60)}s
            </b>
          </span>
          <span>
            HTTP requests <b>{pulse.httpRequests}</b>
          </span>
          <span>
            HTTP 5xx <b>{pulse.httpServerErrors}</b>
          </span>
          <span>
            Dispatch errors <b>{pulse.dispatchErrors}</b>
          </span>
          <span
            title={`Latest ${pulse.sampleCount} successful operations through commit; HTTP excluded. Up to 512 samples, reset on process restart.`}
          >
            Recent operation p95{" "}
            <b>
              {pulse.sampleCount ? `${pulse.recentP95Ms.toFixed(2)} ms` : "—"}
            </b>
          </span>
        </div>
      )}
      <a href="/api/metrics" target="_blank" rel="noreferrer">
        View metrics
      </a>
    </section>
  );
}
