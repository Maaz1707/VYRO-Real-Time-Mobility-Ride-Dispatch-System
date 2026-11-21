"use client";

import { useRef, useState } from "react";
import {
  Check,
  FlaskConical,
  Play,
  RotateCcw,
  ShieldCheck,
} from "lucide-react";

export type Simulation = {
  id: string;
  scenario: string;
  requests: number;
  workers: number;
  assigned: number;
  noDriver: number;
  errors: number;
  p50Ms: number;
  p95Ms: number;
  elapsedMs: number;
  duplicateAssignments: number;
  integrityChecked: boolean;
  rideIds: string[];
  createdAt: string;
};
type Props = {
  runs: Simulation[];
  rides: { id: string; status: string }[];
  connected: boolean;
  busy: boolean;
  onBusyChange: (busy: boolean) => void;
  refresh: () => Promise<void>;
};

export default function SimulationPanel({
  runs,
  rides,
  connected,
  busy,
  onBusyChange,
  refresh,
}: Props) {
  const [count, setCount] = useState(25);
  const [scenario, setScenario] = useState("central");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [running, setRunning] = useState(false);
  const lock = useRef(false);
  const latest = runs[0];
  const active = latest
    ? rides.filter(
        (r) => latest.rideIds.includes(r.id) && r.status === "assigned",
      )
    : [];

  async function simulate() {
    if (lock.current || busy) return;
    lock.current = true;
    onBusyChange(true);
    setRunning(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/simulations", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ requests: count, scenario }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || "Simulation failed.");
      setMessage(
        `Finished ${result.requests} requests with ${result.workers} concurrent workers.`,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Simulation failed.");
    } finally {
      await refresh();
      lock.current = false;
      onBusyChange(false);
      setRunning(false);
    }
  }

  async function release() {
    if (lock.current || busy) return;
    lock.current = true;
    onBusyChange(true);
    setError("");
    setMessage("");
    try {
      const results = await Promise.allSettled(
        active.map(async (r) => {
          const response = await fetch(`/api/rides/${r.id}`, {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ status: "cancelled" }),
          });
          if (!response.ok)
            throw new Error(`Ride ${r.id.slice(0, 6)} could not be released.`);
        }),
      );
      const failed = results.filter((r) => r.status === "rejected").length;
      setMessage(
        `${results.length - failed} rides cancelled. Their drivers are available for another run.`,
      );
      if (failed)
        setError(
          `${failed} rides changed or could not be released. Refresh before retrying.`,
        );
    } catch {
      setError("Could not release drivers.");
    } finally {
      await refresh();
      lock.current = false;
      onBusyChange(false);
    }
  }

  return (
    <section className="simulation-card" id="simulation">
      <div className="card-heading">
        <div>
          <h2>
            <FlaskConical size={18} /> Dispatch laboratory
          </h2>
          <span className="subtle">
            Create competing requests. Inspect the outcome.
          </span>
        </div>
        <span className="experiment-label">BOUNDED EXPERIMENT</span>
      </div>
      <div className="simulation-layout">
        <div className="simulation-controls">
          <label htmlFor="burst-size">Requests per burst</label>
          <select
            id="burst-size"
            value={count}
            disabled={busy}
            onChange={(e) => setCount(+e.target.value)}
          >
            {[10, 25, 50].map((n) => (
              <option key={n} value={n}>
                {n} ride requests
              </option>
            ))}
          </select>
          <label htmlFor="scenario">Demand pattern</label>
          <select
            id="scenario"
            value={scenario}
            disabled={busy}
            onChange={(e) => setScenario(e.target.value)}
          >
            <option value="central">
              Central Bengaluru · fleet contention
            </option>
            <option value="mixed">Mixed coverage · 20% distant pickups</option>
          </select>
          <p>
            Up to 10 workers compete for the current fleet. Assigned drivers
            stay reserved until their rides are completed or cancelled.
          </p>
          <button
            className="primary"
            disabled={busy || !connected}
            onClick={simulate}
          >
            <Play size={16} />
            {running ? "Running experiment…" : "Run burst"}
          </button>
          <div aria-live="polite">
            {message && <p className="feedback">{message}</p>}
            {error && (
              <p className="feedback error" role="alert">
                {error}
              </p>
            )}
          </div>
        </div>
        <div className="simulation-results" aria-live="polite">
          {latest ? (
            <>
              <div className="run-heading">
                <strong>
                  Latest run <span>{latest.id.slice(0, 6).toUpperCase()}</span>
                </strong>
                <time dateTime={latest.createdAt}>
                  {new Date(latest.createdAt).toLocaleTimeString([], {
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </time>
              </div>
              <div
                className="outcome-bar"
                aria-label={`${latest.assigned} assigned, ${latest.noDriver} unavailable, ${latest.errors} errors`}
              >
                {latest.assigned > 0 && (
                  <span
                    className="outcome-assigned"
                    style={{
                      width: `${(latest.assigned / latest.requests) * 100}%`,
                    }}
                  />
                )}
                {latest.noDriver > 0 && (
                  <span
                    className="outcome-unavailable"
                    style={{
                      width: `${(latest.noDriver / latest.requests) * 100}%`,
                    }}
                  />
                )}
                {latest.errors > 0 && (
                  <span
                    className="outcome-error"
                    style={{
                      width: `${(latest.errors / latest.requests) * 100}%`,
                    }}
                  />
                )}
              </div>
              <div className="outcome-counts">
                <span>
                  <b>{latest.assigned}</b> Assigned
                </span>
                <span>
                  <b>{latest.noDriver}</b> No driver
                </span>
                <span>
                  <b>{latest.errors}</b> Errors
                </span>
              </div>
              <div className="timing-grid">
                <div>
                  <span>p50 operation</span>
                  <strong>
                    {latest.p50Ms.toFixed(2)} <small>ms</small>
                  </strong>
                </div>
                <div>
                  <span>p95 operation</span>
                  <strong>
                    {latest.p95Ms.toFixed(2)} <small>ms</small>
                  </strong>
                </div>
                <div>
                  <span>Burst duration</span>
                  <strong>
                    {latest.elapsedMs.toFixed(1)} <small>ms</small>
                  </strong>
                </div>
              </div>
              <div
                className={`integrity ${latest.integrityChecked && latest.duplicateAssignments === 0 ? "passed" : "failed"}`}
              >
                <ShieldCheck size={17} />
                {!latest.integrityChecked
                  ? "Integrity check unavailable"
                  : latest.duplicateAssignments === 0
                    ? "No duplicate active driver assignments"
                    : `${latest.duplicateAssignments} duplicate active assignments detected`}
              </div>
              <p className="measurement-note">
                {latest.requests} requests · {latest.workers} workers ·{" "}
                {latest.scenario === "mixed"
                  ? "mixed coverage"
                  : "central demand"}
                . p50/p95 include successful dispatch operations (assigned and
                no-driver), through commit. HTTP and failed operations are
                excluded. These are local observations.
              </p>
              <button
                className="secondary release"
                disabled={busy || !connected || active.length === 0}
                onClick={release}
              >
                <RotateCcw size={14} /> Release this run’s drivers{" "}
                {active.length > 0 && `(${active.length})`}
              </button>
              <p className="measurement-note">
                Release cancels this run’s active rides. Saved results remain as
                measured at run time.
              </p>
            </>
          ) : (
            <div className="simulation-empty">
              <FlaskConical size={30} />
              <h3>Your fleet. Under pressure.</h3>
              <p>
                Run a burst to see how requests compete for available drivers,
                and whether reservations stay consistent.
              </p>
              <span>
                <Check size={14} /> Results persist in database mode
              </span>
            </div>
          )}
        </div>
      </div>
      {runs.length > 1 && (
        <details className="run-history">
          <summary>Previous experiments ({runs.length - 1})</summary>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>RUN</th>
                  <th>DEMAND</th>
                  <th>REQUESTS</th>
                  <th>ASSIGNED</th>
                  <th>NO DRIVER / ERRORS</th>
                  <th>P95 OPERATION</th>
                </tr>
              </thead>
              <tbody>
                {runs.slice(1, 6).map((r) => (
                  <tr key={r.id}>
                    <td>{r.id.slice(0, 6).toUpperCase()}</td>
                    <td>{r.scenario}</td>
                    <td>{r.requests}</td>
                    <td>{r.assigned}</td>
                    <td>
                      {r.noDriver} / {r.errors}
                    </td>
                    <td>{r.p95Ms.toFixed(2)} ms</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </section>
  );
}
