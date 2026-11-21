"use client";
import { useEffect, useRef, useState } from "react";
import OperationsPanel from "./OperationsPanel";
import SimulationPanel, { type Simulation } from "./SimulationPanel";
import {
  Activity,
  ArrowUpRight,
  CarFront,
  Check,
  CircleDot,
  Clock3,
  Layers,
  MapPin,
  Radio,
  Route,
  X,
  Zap,
} from "lucide-react";

type Point = { lat: number; lng: number };
type Driver = Point & {
  id: string;
  name: string;
  vehicle: string;
  status: string;
};
type Ride = {
  id: string;
  pickup: Point;
  destination: Point;
  status: string;
  driverId: string;
  driverName: string;
  pickupDistanceKm: number;
  etaMinutes: number;
  dispatchMs: number;
  createdAt: string;
};
type State = {
  drivers: Driver[];
  rides: Ride[];
  mode: string;
  simulations: Simulation[];
};
const places = [
  { name: "MG Road", lat: 12.9756, lng: 77.6067 },
  { name: "Cubbon Park", lat: 12.9763, lng: 77.5929 },
  { name: "Indiranagar", lat: 12.9784, lng: 77.6408 },
  { name: "Koramangala", lat: 12.9352, lng: 77.6245 },
  { name: "Whitefield · outside demo fleet", lat: 12.9698, lng: 77.75 },
];
const xy = (p: Point) => ({
  x: ((p.lng - 77.55) / 0.12) * 900,
  y: ((13.015 - p.lat) / 0.1) * 650,
});
const moneyless = (n: number) => (n < 1 ? n.toFixed(2) : n.toFixed(1));
export default function Home() {
  const [state, setState] = useState<State>({
    drivers: [],
    rides: [],
    mode: "",
    simulations: [],
  });
  const [pickup, setPickup] = useState(0);
  const [dest, setDest] = useState(3);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [connected, setConnected] = useState(false);
  const [filter, setFilter] = useState("all");
  const pending = useRef(false);
  async function refresh() {
    try {
      const r = await fetch("/api/state", { cache: "no-store" });
      if (!r.ok)
        throw Error("Fleet service unavailable. Start the Go backend.");
      setState(await r.json());
      setConnected(true);
    } catch {
      setConnected(false);
    }
  }
  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 2000);
    return () => clearInterval(t);
  }, []);
  async function request() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const r = await fetch("/api/rides", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(
          {
            pickup: { lat: places[pickup].lat, lng: places[pickup].lng },
            destination: { lat: places[dest].lat, lng: places[dest].lng },
          },
          (_, v) => v,
        ),
      });
      const data = await r.json();
      if (!r.ok) throw Error(data.error || "Dispatch failed");
      setNotice(
        data.status === "assigned"
          ? `${data.driverName} assigned · estimated ${data.etaMinutes} min pickup`
          : "No available driver within 5 km. Try another pickup.",
      );
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Dispatch failed");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function transition(id: string, status: string) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const r = await fetch(`/api/rides/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
      });
      if (!r.ok) throw Error("Could not update ride. Refresh and try again.");
      setNotice(`Ride ${status}. Driver is available again.`);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Update failed");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  const available = state.drivers.filter(
    (d) => d.status === "available",
  ).length;
  const assigned = state.rides.filter((r) => r.status === "assigned");
  const matched = state.rides.filter((r) => r.driverId);
  const avg = matched.length
    ? matched.reduce((a, r) => a + r.dispatchMs, 0) / matched.length
    : 0;
  const visible = state.rides.filter(
    (r) => filter === "all" || r.status === filter,
  );
  const p = xy(places[pickup]);
  const destination = xy(places[dest]);
  return (
    <div className="shell">
      <aside className="sidebar">
        <a className="brand" href="/">
          v<span>y</span>ro<span className="brand-dot">.</span>
        </a>
        <div className="workspace-label">MOBILITY LAB</div>
        <nav>
          <a className="nav-item active" href="/">
            <Layers size={19} /> Dispatch console
          </a>
          <a className="nav-item" href="#rides">
            <Route size={19} /> Ride activity
          </a>
          <a className="nav-item" href="#simulation">
            <Activity size={19} /> Dispatch laboratory
          </a>
          <a className="nav-item" href="#fleet">
            <CarFront size={19} /> Fleet overview
          </a>
        </nav>
        <div className="sidebar-bottom">
          <div className="prototype">P3</div>
          <div>
            Engineering prototype<small>Phase 03 · Mobility engineering</small>
          </div>
        </div>
      </aside>
      <main>
        <header>
          <div className="breadcrumb">
            Workspace <span>/</span> Dispatch console
          </div>
          <div className={"connection " + (connected ? "online" : "")}>
            <i />
            {connected ? "Dispatch service connected" : "Backend disconnected"}
          </div>
        </header>
        <section className="heading">
          <div>
            <div className="eyebrow">FLEET OPERATIONS</div>
            <h1>A city in motion.</h1>
            <p>One view of your fleet. Every dispatch, accounted for.</p>
          </div>
          <div className="city">
            <MapPin size={16} /> Bengaluru <span>SIMULATED FLEET</span>
          </div>
        </section>
        <section className="stats" id="fleet">
          {[
            {
              label: "Available drivers",
              value: available.toString().padStart(2, "0"),
              sub: `of ${state.drivers.length} vehicles in fleet`,
              icon: CarFront,
            },
            {
              label: "Active rides",
              value: assigned.length.toString().padStart(2, "0"),
              sub: "Drivers currently reserved",
              icon: Route,
            },
            {
              label: "Avg. dispatch time",
              value: matched.length ? moneyless(avg) : "—",
              unit: matched.length ? "ms" : "",
              sub: "Server-side selection & reservation",
              icon: Zap,
            },
            {
              label: "Assignment rate",
              value: state.rides.length
                ? Math.round((matched.length / state.rides.length) * 100)
                : "—",
              unit: state.rides.length ? "%" : "",
              sub: `${matched.length} matched / ${state.rides.length} requests`,
              icon: Activity,
            },
          ].map((s) => (
            <article className="stat" key={s.label}>
              <div className="stat-top">
                {s.label}
                <s.icon size={18} />
              </div>
              <div className="stat-value">
                {s.value}
                <span>{s.unit}</span>
              </div>
              <div className="stat-sub">{s.sub}</div>
            </article>
          ))}
        </section>
        <OperationsPanel />
        <section className="dispatch-grid">
          <article className="map-card">
            <div className="card-heading">
              <div>
                <h2>Fleet map</h2>
                <span className="subtle">
                  Bengaluru central · schematic view
                </span>
              </div>
              <div className="live">
                <Radio size={14} /> POLLING · 2s
              </div>
            </div>
            <div className="map">
              <svg
                viewBox="0 0 900 650"
                role="img"
                aria-label="Schematic fleet map showing available and busy driver locations"
              >
                <defs>
                  <pattern
                    id="blocks"
                    width="85"
                    height="65"
                    patternTransform="rotate(-18)"
                    patternUnits="userSpaceOnUse"
                  >
                    <rect width="85" height="65" fill="#edf0e9" />
                    <rect
                      x="5"
                      y="5"
                      width="70"
                      height="50"
                      rx="5"
                      fill="#e5e9e0"
                    />
                    <path d="M0 0H85M0 0V65" stroke="#fff" strokeWidth="7" />
                  </pattern>
                </defs>
                <rect width="900" height="650" fill="url(#blocks)" />
                <path
                  d="M100 0L280 170L200 290L380 400L460 650M0 400L180 340L360 230L600 220L900 120M0 130L260 240L560 450L900 520"
                  fill="none"
                  stroke="#d5dace"
                  strokeWidth="24"
                />
                <path
                  d="M100 0L280 170L200 290L380 400L460 650M0 400L180 340L360 230L600 220L900 120M0 130L260 240L560 450L900 520"
                  fill="none"
                  stroke="#fffef8"
                  strokeWidth="17"
                />
                <path
                  d="M630 0Q550 90 650 160T730 370T850 650"
                  fill="none"
                  stroke="#c8dce2"
                  strokeWidth="26"
                />
                <path
                  d="M300 170l70-35 45 85-85 45zM120 440l100-15 25 90-100 20z"
                  fill="#cbdcbd"
                />
                <g className="map-labels">
                  <text x="280" y="195">
                    CUBBON PARK
                  </text>
                  <text x="430" y="300">
                    MG ROAD
                  </text>
                  <text x="650" y="280">
                    INDIRANAGAR
                  </text>
                  <text x="490" y="555">
                    KORAMANGALA
                  </text>
                  <text x="100" y="100">
                    VASANTH NAGAR
                  </text>
                  <text x="100" y="580">
                    LALBAGH
                  </text>
                </g>
                {assigned.map((r) => {
                  const a = xy(r.pickup),
                    b = xy(r.destination);
                  return (
                    <path
                      key={r.id}
                      d={`M${a.x} ${a.y} Q${a.x + 90} ${b.y - 70} ${b.x} ${b.y}`}
                      fill="none"
                      stroke="#78983e"
                      strokeWidth="2"
                      strokeDasharray="7 6"
                    />
                  );
                })}
                {state.drivers.map((d) => {
                  const pt = xy(d);
                  return (
                    <g key={d.id} transform={`translate(${pt.x},${pt.y})`}>
                      <title>
                        {d.name} · {d.status} · {d.vehicle}
                      </title>
                      <rect
                        x="-15"
                        y="-15"
                        width="30"
                        height="30"
                        rx="10"
                        fill={d.status === "available" ? "#263f31" : "#9b7948"}
                        stroke="white"
                        strokeWidth="3"
                      />
                      <path
                        d="M-5 3V-4Q-5-6-3-6H3Q5-6 5-4V3ZM-6 0H6M-3 4V6M3 4V6"
                        fill="none"
                        stroke="white"
                        strokeWidth="1.6"
                      />
                    </g>
                  );
                })}
                <circle cx={p.x} cy={p.y} r="25" fill="#b1df4c" opacity=".22" />
                <circle
                  cx={p.x}
                  cy={p.y}
                  r="9"
                  fill="#9ac738"
                  stroke="white"
                  strokeWidth="4"
                />
                {destination.x < 900 && (
                  <g>
                    <circle
                      cx={destination.x}
                      cy={destination.y}
                      r="9"
                      fill="#a56a50"
                      stroke="white"
                      strokeWidth="4"
                    />
                  </g>
                )}
              </svg>
              <div className="map-note">
                Illustrative streets · real coordinate matching
              </div>
              <div className="legend">
                <span>
                  <i className="available" />
                  Available
                </span>
                <span>
                  <i className="reserved" />
                  Reserved
                </span>
                <span>
                  <i className="pickup" />
                  Pickup
                </span>
              </div>
            </div>
          </article>
          <article className="request-card">
            <div className="card-heading">
              <div>
                <h2>Dispatch a ride</h2>
                <span className="subtle">Put the matching engine to work.</span>
              </div>
              <div className="icon-box">
                <ArrowUpRight size={20} />
              </div>
            </div>
            <div className="request-body">
              <div className="route-form">
                <label htmlFor="pickup">
                  <CircleDot size={16} /> Pickup location
                </label>
                <select
                  id="pickup"
                  value={pickup}
                  onChange={(e) => setPickup(+e.target.value)}
                >
                  {places.map((v, i) => (
                    <option value={i} key={v.name}>
                      {v.name}
                    </option>
                  ))}
                </select>
                <label htmlFor="dest">
                  <MapPin size={16} /> Destination
                </label>
                <select
                  id="dest"
                  value={dest}
                  onChange={(e) => setDest(+e.target.value)}
                >
                  {places.map((v, i) => (
                    <option value={i} key={v.name}>
                      {v.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="policy">
                <div>
                  <Radio size={16} />
                  <strong>Nearest available driver</strong>
                </div>
                <p>
                  Search within 5 km. Reserve one driver atomically. Return an
                  estimated pickup time.
                </p>
                <span>25 km/h assumed speed · straight-line distance</span>
              </div>
              <button
                className="primary"
                onClick={request}
                disabled={busy || !connected || pickup === dest}
              >
                <Zap size={17} />
                {busy ? "Processing…" : "Find a driver"}
              </button>
              {pickup === dest && (
                <p className="feedback">Choose a different destination.</p>
              )}
              <div aria-live="polite">
                {notice && <p className="feedback">{notice}</p>}
                {error && (
                  <p className="feedback error" role="alert">
                    {error}
                  </p>
                )}
              </div>
              <div className="storage">
                <span className="storage-dot" />
                {state.mode === "postgres"
                  ? "PostgreSQL + PostGIS"
                  : "In-memory demo"}
                <span>
                  {state.mode === "postgres"
                    ? "Persistent state"
                    : "Resets on backend restart"}
                </span>
              </div>
            </div>
          </article>
        </section>
        <SimulationPanel
          runs={state.simulations || []}
          rides={state.rides}
          connected={connected}
          busy={busy}
          onBusyChange={(v) => {
            pending.current = v;
            setBusy(v);
          }}
          refresh={refresh}
        />
        <section className="rides-card" id="rides">
          <div className="card-heading">
            <div>
              <h2>
                Ride activity{" "}
                <span className="count">{state.rides.length}</span>
              </h2>
              <span className="subtle">
                Live dispatch outcomes and ride lifecycle.
              </span>
            </div>
            <select
              aria-label="Filter rides"
              className="filter"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            >
              <option value="all">All requests</option>
              <option value="assigned">Assigned</option>
              <option value="completed">Completed</option>
              <option value="cancelled">Cancelled</option>
              <option value="no_driver">No driver</option>
            </select>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>RIDE</th>
                  <th>DRIVER</th>
                  <th>STATUS</th>
                  <th>PICKUP ESTIMATE</th>
                  <th>DISPATCH</th>
                  <th>ACTIONS</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((r) => (
                  <tr key={r.id}>
                    <td>
                      <strong>VY-{r.id.slice(0, 6).toUpperCase()}</strong>
                      <small>
                        {new Date(r.createdAt).toLocaleTimeString([], {
                          hour: "2-digit",
                          minute: "2-digit",
                          second: "2-digit",
                        })}
                      </small>
                    </td>
                    <td>{r.driverName || "—"}</td>
                    <td>
                      <span className={`badge ${r.status}`}>
                        {r.status.replace("_", " ")}
                      </span>
                    </td>
                    <td>
                      {r.driverId ? (
                        <>
                          <Clock3 size={13} /> {r.etaMinutes} min{" "}
                          <span className="subtle">
                            · {r.pickupDistanceKm} km
                          </span>
                        </>
                      ) : (
                        "Outside availability"
                      )}
                    </td>
                    <td>{r.dispatchMs.toFixed(2)} ms</td>
                    <td>
                      {r.status === "assigned" ? (
                        <div className="actions">
                          <button
                            disabled={busy}
                            title="Complete ride"
                            onClick={() => transition(r.id, "completed")}
                          >
                            <Check size={14} />
                            Complete
                          </button>
                          <button
                            disabled={busy}
                            aria-label={`Cancel ride ${r.id.slice(0, 6)}`}
                            onClick={() => transition(r.id, "cancelled")}
                          >
                            <X size={14} />
                          </button>
                        </div>
                      ) : (
                        <span className="subtle">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!visible.length && (
              <div className="empty">
                <Route size={24} />
                <strong>
                  {state.rides.length
                    ? "No rides in this view"
                    : "Your next dispatch starts here"}
                </strong>
                <span>
                  {state.rides.length
                    ? "Choose another status to see activity."
                    : "Choose a pickup and destination to see the matching engine in action."}
                </span>
              </div>
            )}
          </div>
        </section>
        <footer>
          <span>VYRO / MOBILITY ENGINEERING LAB</span>
          <span>Prototype · simulated fleet · no real bookings</span>
        </footer>
      </main>
    </div>
  );
}
