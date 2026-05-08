#!/usr/bin/env python3
"""
Parallel HTTP load against booking + queue (garage-blocked lanes).

- One shared JWT (login once).
- Many worker threads hammer POST /bookings (1 seat) on the same destination (contention).
- Optional GET /queue interleaved (read load).
- Tracks ok/fail, latency percentiles, error strings.

Usage:
  python3 scripts/parallel_load_test.py

Env:
  WASLA_AUTH_URL, WASLA_QUEUE_URL, WASLA_BOOK_URL  (defaults localhost 8001/8002/8003)
  PL_WORKERS=20          concurrent threads (raise slowly; Postgres max_connections / pool may cap you)
  PL_SECONDS=25          run duration per phase
  PL_CARS=24             vehicles in the hot destination
  PL_CAPACITY=5          seats per vehicle
  PL_BLOCK_MOD=6         garage-block every Nth queue position (0 = no blocks)
  PL_READ_RATIO=0.15     fraction of worker iterations that GET queue instead of book
"""
from __future__ import annotations

import json
import os
import random
import threading
import time
import urllib.error
import urllib.request
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Optional

AUTH = os.environ.get("WASLA_AUTH_URL", "http://localhost:8001/api/v1").rstrip("/")
QUEUE = os.environ.get("WASLA_QUEUE_URL", "http://localhost:8002/api/v1").rstrip("/")
BOOK = os.environ.get("WASLA_BOOK_URL", "http://localhost:8003/api/v1").rstrip("/")

WORKERS = int(os.environ.get("PL_WORKERS", "20"))
SECONDS = float(os.environ.get("PL_SECONDS", "22"))
CARS = int(os.environ.get("PL_CARS", "24"))
CAPACITY = int(os.environ.get("PL_CAPACITY", "5"))
BLOCK_MOD = int(os.environ.get("PL_BLOCK_MOD", "6"))
READ_RATIO = float(os.environ.get("PL_READ_RATIO", "0.12"))


def http_json(
    method: str,
    url: str,
    token: Optional[str] = None,
    body: Optional[dict[str, Any]] = None,
) -> tuple[int, dict[str, Any]]:
    payload = json.dumps(body).encode() if body else None
    hdrs = {"Content-Type": "application/json"}
    if token:
        hdrs["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=payload, headers=hdrs, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw.strip() else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"message": raw, "_raw": True}
    except Exception as e:
        return 0, {"message": str(e), "_raw": True}


def login() -> str:
    code, data = http_json("POST", f"{AUTH}/auth/login", body={"cin": "12345678"})
    if code != 200 or not data.get("success"):
        raise SystemExit(f"login {code}: {data}")
    return str((data.get("data") or {})["token"])


def create_route(token: str, station_id: str, name: str) -> None:
    code, data = http_json(
        "POST",
        f"{QUEUE}/routes",
        token,
        {"stationId": station_id, "stationName": name, "basePrice": 7.0},
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"route: {code} {data}")


def clear_queue(token: str, station_id: str) -> None:
    http_json("DELETE", f"{QUEUE}/queue/{station_id}/clear", token)


def unique_plate_strict() -> str:
    """High-entropy plate; avoids collisions when the DB already has many test vehicles."""
    h = uuid.uuid4().hex
    a = 10 + (int(h[:2], 16) % 90)
    tail = int(h[2:6], 16) % 10000
    return f"{a:02d} TUN {tail:04d}"


def vehicle_create(token: str, cap: int) -> str:
    code, data = http_json(
        "POST",
        f"{QUEUE}/vehicles",
        token,
        {"licensePlate": unique_plate_strict(), "capacity": cap},
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"vehicle: {code} {data}")
    return str((data.get("data") or {})["id"])


def enqueue(token: str, station_id: str, name: str, vehicle_id: str) -> dict[str, Any]:
    code, data = http_json(
        "POST",
        f"{QUEUE}/queue/{station_id}",
        token,
        {
            "vehicleId": vehicle_id,
            "destinationId": station_id,
            "destinationName": name,
        },
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"enqueue: {code} {data}")
    return (data.get("data") or {})["queueEntry"]


def list_queue(token: str, station_id: str) -> list[dict]:
    code, data = http_json("GET", f"{QUEUE}/queue/{station_id}", token)
    if code != 200 or not data.get("success"):
        return []
    return list(data.get("data") or [])


def set_block(token: str, station_id: str, entry_id: str, blocked: bool) -> None:
    code, data = http_json(
        "PUT",
        f"{QUEUE}/queue/{station_id}/entry/{entry_id}/garage-block",
        token,
        {"blocked": blocked},
    )
    if code != 200 or not data.get("success"):
        raise SystemExit(f"block: {code} {data}")


def book_dest(token: str, station_id: str, key: str) -> tuple[int, dict[str, Any], float]:
    t0 = time.perf_counter()
    code, data = http_json(
        "POST",
        f"{BOOK}/bookings",
        token,
        {
            "destinationId": station_id,
            "seats": 1,
            "preferExactFit": False,
            "idempotencyKey": key,
        },
    )
    return code, data, time.perf_counter() - t0


class Stats:
    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.ok = 0
        self.fail = 0
        self.lat_ms: list[float] = []
        self.err_msg: dict[str, int] = {}

    def add(self, ok: bool, dt_s: float, err: str) -> None:
        with self.lock:
            if ok:
                self.ok += 1
            else:
                self.fail += 1
                self.err_msg[err[:120]] = self.err_msg.get(err[:120], 0) + 1
            self.lat_ms.append(dt_s * 1000.0)


def pct(sorted_vals: list[float], p: float) -> float:
    if not sorted_vals:
        return 0.0
    i = int(round((len(sorted_vals) - 1) * p))
    return sorted_vals[max(0, min(i, len(sorted_vals) - 1))]


def worker(
    wid: int,
    token: str,
    station_id: str,
    blocked_v: frozenset[str],
    end: float,
    stats: Stats,
) -> None:
    r = random.Random(wid * 7919)
    dry_miss = 0
    while time.time() < end:
        if r.random() < READ_RATIO:
            t0 = time.perf_counter()
            code, data = http_json("GET", f"{QUEUE}/queue/{station_id}", token)
            dt = time.perf_counter() - t0
            ok = code == 200 and data.get("success")
            rows = list(data.get("data") or [])
            if ok:
                for row in rows:
                    if row.get("isGarageBlocked") and str(row.get("vehicleId")) in blocked_v:
                        pass
            stats.add(ok, dt, "" if ok else str(data.get("error") or data.get("message") or f"queue_{code}"))
            continue
        code, data, dt = book_dest(token, station_id, f"pl-{wid}-{time.time()}-{r.random()}")
        ok = code == 201 and data.get("success")
        if not ok:
            msg = str(data.get("error") or data.get("message") or f"http_{code}")
            stats.add(False, dt, msg)
            if "no vehicle" in msg.lower():
                dry_miss += 1
                if dry_miss >= 8:
                    return
            continue
        vid = str((data.get("data") or {}).get("vehicleId", ""))
        if vid in blocked_v:
            stats.add(False, dt, "invariant: booked blocked vehicle")
            continue
        dry_miss = 0
        stats.add(True, dt, "")


def main() -> None:
    print(
        f"Parallel load: workers={WORKERS} duration={SECONDS}s cars={CARS} "
        f"cap={CAPACITY} block_mod={BLOCK_MOD} read_ratio={READ_RATIO}"
    )
    token = login()
    time.sleep(2.0)

    sid = f"pl_{uuid.uuid4().hex[:12]}"
    name = "Parallel load lane"
    create_route(token, sid, name)
    clear_queue(token, sid)
    for _ in range(CARS):
        vid = vehicle_create(token, CAPACITY)
        enqueue(token, sid, name, vid)

    rows = sorted(list_queue(token, sid), key=lambda r: int(r["queuePosition"]))
    blocked_ids: set[str] = set()
    if BLOCK_MOD > 0:
        for i, row in enumerate(rows, start=1):
            if i % BLOCK_MOD == 0:
                set_block(token, sid, str(row["id"]), True)
                blocked_ids.add(str(row["vehicleId"]))
    frozen_blocked = frozenset(blocked_ids)
    bookable = sum(
        int(r["availableSeats"])
        for r in list_queue(token, sid)
        if not r.get("isGarageBlocked")
    )
    print(f"  destination={sid} blocked_vehicles={len(frozen_blocked)} bookable_seats≈{bookable}")

    stats = Stats()
    end = time.time() + SECONDS
    with ThreadPoolExecutor(max_workers=WORKERS) as ex:
        futs = [
            ex.submit(worker, w, token, sid, frozen_blocked, end, stats)
            for w in range(WORKERS)
        ]
        for f in as_completed(futs):
            f.result()

    lat = sorted(stats.lat_ms)
    print("")
    print(f"Results: ok={stats.ok} fail={stats.fail} total={stats.ok + stats.fail}")
    if lat:
        print(
            f"Latency ms: min={lat[0]:.1f} p50={pct(lat, 0.50):.1f} "
            f"p95={pct(lat, 0.95):.1f} max={lat[-1]:.1f}"
        )
    if stats.err_msg:
        print("Top errors:")
        for msg, n in sorted(stats.err_msg.items(), key=lambda x: -x[1])[:12]:
            print(f"  {n}x  {msg}")

    # Invariant: never booked a blocked vehicle (worker would count as fail)
    inv = stats.err_msg.get("invariant: booked blocked vehicle", 0)
    if inv:
        raise SystemExit(f"INVARIANT VIOLATION blocked bookings={inv}")
    if any("deadlock" in k.lower() for k in stats.err_msg):
        print("")
        print(
            "*** Many PostgreSQL deadlocks seen — rebuild and RESTART `booking-service` from current "
            "source (per-destination advisory lock), then re-run. Old binaries will contend on vehicle_queue."
        )
    if any("too many clients" in k.lower() or "remaining connection" in k.lower() for k in stats.err_msg):
        print("")
        print(
            "*** Postgres connection limit hit — lower PL_WORKERS or raise max_connections / pool size "
            "on booking-service."
        )
    print("")
    print("Parallel load finished (no blocked-vehicle invariant violations).")


if __name__ == "__main__":
    main()
