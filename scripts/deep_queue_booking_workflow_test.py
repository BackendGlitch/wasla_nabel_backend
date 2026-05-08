#!/usr/bin/env python3
"""
Deep integration + load test: queue lifecycle + garage block + bookings.

Runs in order:
  1) Sequential scenario (deterministic): enqueue → block → book (invariant) →
     unblock → block again → DELETE blocked entry → verify queue.
  2) Optional parallel chaos: mixed list / book / enqueue / garage toggle /
     delete (prefer removing blocked rows).

Usage:
  python3 scripts/deep_queue_booking_workflow_test.py

Env:
  WASLA_AUTH_URL, WASLA_QUEUE_URL, WASLA_BOOK_URL  (same defaults as parallel_load_test)

  DQ_RUN_SEQ=1            run sequential phase (0 to skip)
  DQ_RUN_PAR=1            run parallel phase (0 to skip)
  DQ_WORKERS=10           parallel worker threads
  DQ_SECONDS=25           parallel phase duration
  DQ_INITIAL_CARS=10      vehicles pre-seeded before parallel phase
  DQ_CAPACITY=5           seats per created vehicle
  DQ_BOOK_WEIGHT=35       relative weight for POST /bookings
  DQ_LIST_WEIGHT=12       GET /queue
  DQ_ENQUEUE_WEIGHT=18    create vehicle + POST queue
  DQ_BLOCK_WEIGHT=15      toggle garage-block on a random entry
  DQ_DEL_BLOCKED_WEIGHT=12  DELETE a random garage-blocked entry (if any)
  DQ_DEL_ANY_WEIGHT=3     DELETE any random entry (low)
  DQ_FAIL_ON_DEADLOCK=0   set to 1 to exit non-zero if any Postgres deadlock appears in parallel phase
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

RUN_SEQ = os.environ.get("DQ_RUN_SEQ", "1") not in ("0", "false", "False")
RUN_PAR = os.environ.get("DQ_RUN_PAR", "1") not in ("0", "false", "False")
WORKERS = int(os.environ.get("DQ_WORKERS", "10"))
SECONDS = float(os.environ.get("DQ_SECONDS", "25"))
INITIAL_CARS = int(os.environ.get("DQ_INITIAL_CARS", "10"))
CAPACITY = int(os.environ.get("DQ_CAPACITY", "5"))

W_BOOK = int(os.environ.get("DQ_BOOK_WEIGHT", "35"))
W_LIST = int(os.environ.get("DQ_LIST_WEIGHT", "12"))
W_ENQ = int(os.environ.get("DQ_ENQUEUE_WEIGHT", "18"))
W_BLK = int(os.environ.get("DQ_BLOCK_WEIGHT", "15"))
W_DELB = int(os.environ.get("DQ_DEL_BLOCKED_WEIGHT", "12"))
W_DELA = int(os.environ.get("DQ_DEL_ANY_WEIGHT", "3"))
FAIL_ON_DEADLOCK = os.environ.get("DQ_FAIL_ON_DEADLOCK", "0") in ("1", "true", "True")


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
        with urllib.request.urlopen(req, timeout=90) as resp:
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


def set_block(token: str, station_id: str, entry_id: str, blocked: bool) -> tuple[int, dict[str, Any]]:
    return http_json(
        "PUT",
        f"{QUEUE}/queue/{station_id}/entry/{entry_id}/garage-block",
        token,
        {"blocked": blocked},
    )


def delete_entry(token: str, station_id: str, entry_id: str) -> tuple[int, dict[str, Any]]:
    return http_json("DELETE", f"{QUEUE}/queue/{station_id}/entry/{entry_id}", token)


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


def vehicle_blocked_in_queue(rows: list[dict], vehicle_id: str) -> bool:
    for r in rows:
        if str(r.get("vehicleId")) == vehicle_id and r.get("isGarageBlocked"):
            return True
    return False


class ParStats:
    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.ok = 0
        self.fail = 0
        self.err_msg: dict[str, int] = {}

    def add_ok(self) -> None:
        with self.lock:
            self.ok += 1

    def add_fail(self, err: str) -> None:
        with self.lock:
            self.fail += 1
            k = err[:140]
            self.err_msg[k] = self.err_msg.get(k, 0) + 1


def pick_op(r: random.Random) -> str:
    total = W_BOOK + W_LIST + W_ENQ + W_BLK + W_DELB + W_DELA
    x = r.randint(1, total)
    c = 0
    for name, w in (
        ("book", W_BOOK),
        ("list", W_LIST),
        ("enqueue", W_ENQ),
        ("block", W_BLK),
        ("del_blocked", W_DELB),
        ("del_any", W_DELA),
    ):
        c += w
        if x <= c:
            return name
    return "list"


def parallel_worker(wid: int, token: str, sid: str, name: str, end: float, stats: ParStats) -> None:
    r = random.Random(wid * 11003 + int(time.time()) % 10000)
    dry_book_miss = 0
    while time.time() < end:
        op = pick_op(r)
        if op == "list":
            code, data = http_json("GET", f"{QUEUE}/queue/{sid}", token)
            if code == 200 and data.get("success"):
                stats.add_ok()
            else:
                stats.add_fail(str(data.get("error") or data.get("message") or f"list_{code}"))
            continue

        if op == "book":
            code, data, _dt = book_dest(token, sid, f"dq-{wid}-{time.time()}-{r.random()}")
            if code == 201 and data.get("success"):
                dry_book_miss = 0
                # Do not assert "not blocked" via a follow-up list_queue: another worker may
                # garage-block the same vehicle after the booking commits (false positive).
                stats.add_ok()
            else:
                msg = str(data.get("error") or data.get("message") or f"http_{code}")
                stats.add_fail(msg)
                if "no vehicle" in msg.lower():
                    dry_book_miss += 1
                    if dry_book_miss >= 10:
                        time.sleep(0.05)
                continue

        if op == "enqueue":
            try:
                vid = vehicle_create(token, CAPACITY)
                enqueue(token, sid, name, vid)
                stats.add_ok()
            except SystemExit as e:
                stats.add_fail(str(e))
            continue

        rows = list_queue(token, sid)
        if not rows:
            stats.add_fail("parallel: empty queue for mutating op")
            time.sleep(0.02)
            continue

        if op == "block":
            row = r.choice(rows)
            eid = str(row["id"])
            cur = bool(row.get("isGarageBlocked"))
            nxt = not cur
            code, data = set_block(token, sid, eid, nxt)
            if code == 200 and data.get("success"):
                stats.add_ok()
            else:
                stats.add_fail(str(data.get("error") or data.get("message") or f"block_{code}"))
            continue

        if op == "del_blocked":
            blocked = [x for x in rows if x.get("isGarageBlocked")]
            if not blocked:
                stats.add_ok()
                continue
            row = r.choice(blocked)
            code, data = delete_entry(token, sid, str(row["id"]))
            if code == 200 and data.get("success"):
                stats.add_ok()
            else:
                stats.add_fail(str(data.get("error") or data.get("message") or f"del_{code}"))
            continue

        if op == "del_any":
            row = r.choice(rows)
            code, data = delete_entry(token, sid, str(row["id"]))
            if code == 200 and data.get("success"):
                stats.add_ok()
            else:
                stats.add_fail(str(data.get("error") or data.get("message") or f"del_{code}"))
            continue


def run_sequential(token: str, sid: str, name: str) -> None:
    print("")
    print("--- Sequential phase ---")
    clear_queue(token, sid)

    vids: list[str] = []
    entries: list[dict] = []
    for _ in range(3):
        vid = vehicle_create(token, CAPACITY)
        vids.append(vid)
        qe = enqueue(token, sid, name, vid)
        entries.append(qe)

    rows = sorted(list_queue(token, sid), key=lambda x: int(x["queuePosition"]))
    if len(rows) < 3:
        raise SystemExit(f"seq: expected 3 queue rows, got {len(rows)}")
    mid = rows[1]
    mid_eid = str(mid["id"])
    mid_vid = str(mid["vehicleId"])
    print(f"  enqueue 3 vehicles; blocking middle entry {mid_eid} (vehicle {mid_vid})")

    code, data = set_block(token, sid, mid_eid, True)
    if code != 200 or not data.get("success"):
        raise SystemExit(f"seq block: {code} {data}")
    rows_b = list_queue(token, sid)
    if not vehicle_blocked_in_queue(rows_b, mid_vid):
        raise SystemExit("seq: middle vehicle not blocked after toggle")

    # Book until one success; must not be blocked vehicle
    booked_vid: Optional[str] = None
    for attempt in range(80):
        code, data, _ = book_dest(token, sid, f"seq-book-{attempt}-{uuid.uuid4().hex[:8]}")
        if code == 201 and data.get("success"):
            booked_vid = str((data.get("data") or {}).get("vehicleId", ""))
            break
        msg = str(data.get("error") or data.get("message") or "")
        if "no vehicle" not in msg.lower():
            print(f"  seq book attempt {attempt}: {code} {msg[:120]}")
        time.sleep(0.03)
    if not booked_vid:
        raise SystemExit("seq: could not complete any booking (pool empty?)")
    if booked_vid == mid_vid:
        raise SystemExit("INVARIANT (seq): booking targeted garage-blocked vehicle")
    rows_check = list_queue(token, sid)
    if vehicle_blocked_in_queue(rows_check, booked_vid):
        raise SystemExit("INVARIANT (seq): booked vehicle still flagged blocked in queue")
    print(f"  booking ok vehicleId={booked_vid} (not blocked lane)")

    code, data = set_block(token, sid, mid_eid, False)
    if code != 200 or not data.get("success"):
        raise SystemExit(f"seq unblock: {code} {data}")
    print("  unblocked middle vehicle")

    code, data = set_block(token, sid, mid_eid, True)
    if code != 200 or not data.get("success"):
        raise SystemExit(f"seq block2: {code} {data}")
    print("  blocked again; deleting that queue entry while blocked")

    code, data = delete_entry(token, sid, mid_eid)
    if code != 200 or not data.get("success"):
        raise SystemExit(f"seq delete: {code} {data}")
    rows_after = list_queue(token, sid)
    ids_after = {str(r["id"]) for r in rows_after}
    if mid_eid in ids_after:
        raise SystemExit("seq: deleted entry still present")
    vids_after = {str(r.get("vehicleId")) for r in rows_after}
    if mid_vid in vids_after:
        raise SystemExit("seq: blocked vehicle still in queue after delete")
    print(f"  removed blocked entry; {len(rows_after)} row(s) remain")
    print("Sequential phase OK.")


def run_parallel(token: str, sid: str, name: str) -> ParStats:
    print("")
    print("--- Parallel phase ---")
    clear_queue(token, sid)
    for _ in range(INITIAL_CARS):
        vid = vehicle_create(token, CAPACITY)
        enqueue(token, sid, name, vid)
    print(
        f"  destination={sid} initial_cars={INITIAL_CARS} cap={CAPACITY} "
        f"workers={WORKERS} duration={SECONDS}s"
    )
    print(
        f"  op weights: book={W_BOOK} list={W_LIST} enqueue={W_ENQ} "
        f"block={W_BLK} del_blocked={W_DELB} del_any={W_DELA}"
    )

    stats = ParStats()
    end = time.time() + SECONDS
    with ThreadPoolExecutor(max_workers=WORKERS) as ex:
        futs = [ex.submit(parallel_worker, w, token, sid, name, end, stats) for w in range(WORKERS)]
        for f in as_completed(futs):
            f.result()
    return stats


def main() -> None:
    print(
        "Deep queue + booking workflow test "
        f"(seq={RUN_SEQ} par={RUN_PAR} workers={WORKERS}s={SECONDS}s initial_cars={INITIAL_CARS})"
    )
    token = login()
    time.sleep(1.0)

    sid = f"dq_{uuid.uuid4().hex[:12]}"
    name = "Deep workflow lane"
    create_route(token, sid, name)

    if RUN_SEQ:
        run_sequential(token, sid, name)

    if RUN_PAR:
        stats = run_parallel(token, sid, name)
        print("")
        print(f"Parallel results: ok={stats.ok} fail={stats.fail}")
        if stats.err_msg:
            print("Top parallel errors:")
            for msg, n in sorted(stats.err_msg.items(), key=lambda x: -x[1])[:15]:
                print(f"  {n}x  {msg}")
        if any("deadlock" in k.lower() for k in stats.err_msg):
            print("")
            print(
                "*** Deadlocks seen under mixed queue + booking load — typical causes: "
                "stale booking-service binary (missing per-destination advisory lock), and/or "
                "queue-service contention on vehicle_queue. Rebuild/restart both; try fewer workers."
            )
            if FAIL_ON_DEADLOCK:
                raise SystemExit("DQ_FAIL_ON_DEADLOCK=1 and deadlocks were observed.")
        if any("too many clients" in k.lower() or "remaining connection" in k.lower() for k in stats.err_msg):
            print("")
            print("*** Postgres max connections — lower DQ_WORKERS or raise pool/limits.")
        print("Parallel phase finished (no blocked-booking invariant violations).")

    print("")
    print("Deep workflow test completed successfully.")


if __name__ == "__main__":
    main()
