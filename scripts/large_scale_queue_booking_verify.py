#!/usr/bin/env python3
"""
Large-scale HTTP verification for queue + destination booking + garage block.

- Many destinations in parallel
- Many vehicles / queue rows per destination
- Hundreds of single-seat (and some multi-seat) bookings
- Invariant: no booking is ever allocated to a garage-blocked row's vehicle
- Optional: serving/priority smoke on a dedicated lane

Requires: auth + queue + booking + Redis + DB migration 027+ (serving table optional for block-skip only).

  python3 scripts/large_scale_queue_booking_verify.py

Env: WASLA_AUTH_URL, WASLA_QUEUE_URL, WASLA_BOOKING_URL (same defaults as other scripts).
"""
from __future__ import annotations

import json
import os
import random
import time
import urllib.error
import urllib.request
import uuid
from typing import Any, Optional

AUTH = os.environ.get("WASLA_AUTH_URL", "http://localhost:8001/api/v1").rstrip("/")
QUEUE = os.environ.get("WASLA_QUEUE_URL", "http://localhost:8002/api/v1").rstrip("/")
BOOK = os.environ.get("WASLA_BOOK_URL", "http://localhost:8003/api/v1").rstrip("/")

NUM_DESTINATIONS = int(os.environ.get("LS_NUM_DEST", "10"))
CARS_PER_DEST = int(os.environ.get("LS_CARS_PER_DEST", "22"))
CAPACITY = int(os.environ.get("LS_CAPACITY", "5"))
BOOKINGS_PER_DEST = int(os.environ.get("LS_BOOKINGS_PER_DEST", "95"))
MULTI_SEAT_BOOKINGS = int(os.environ.get("LS_MULTI_BOOKINGS", "40"))
RNG_SEED = int(os.environ.get("LS_SEED", "42"))

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
        with urllib.request.urlopen(req, timeout=180) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw.strip() else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"message": raw, "_raw": True}


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
        {"stationId": station_id, "stationName": name, "basePrice": 6.5},
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"route {station_id}: {code} {data}")


def clear_queue(token: str, station_id: str) -> None:
    http_json("DELETE", f"{QUEUE}/queue/{station_id}/clear", token)


def unique_plate() -> str:
    # NN TUN NNNN — unique across script runs (avoids DB plate collisions from prior tests).
    u = uuid.uuid4().int
    a = 10 + (u % 90)
    tail = (u // 90) % 10000
    return f"{a:02d} TUN {tail:04d}"


def vehicle_create(token: str, cap: int) -> str:
    plate = unique_plate()
    code, data = http_json(
        "POST",
        f"{QUEUE}/vehicles",
        token,
        {"licensePlate": plate, "capacity": cap},
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"vehicle {plate}: {code} {data}")
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


def bookable_seats(token: str, station_id: str) -> int:
    return sum(
        int(r["availableSeats"])
        for r in list_queue(token, station_id)
        if not r.get("isGarageBlocked")
    )


def list_queue(token: str, station_id: str) -> list[dict]:
    code, data = http_json("GET", f"{QUEUE}/queue/{station_id}", token)
    if code != 200 or not data.get("success"):
        raise SystemExit(f"list {station_id}: {code} {data}")
    return list(data.get("data") or [])


def set_block(token: str, station_id: str, entry_id: str, blocked: bool) -> None:
    code, data = http_json(
        "PUT",
        f"{QUEUE}/queue/{station_id}/entry/{entry_id}/garage-block",
        token,
        {"blocked": blocked},
    )
    if code != 200 or not data.get("success"):
        raise SystemExit(f"block {blocked}: {code} {data}")


def book_dest(token: str, station_id: str, seats: int, key: str) -> tuple[int, dict[str, Any]]:
    return http_json(
        "POST",
        f"{BOOK}/bookings",
        token,
        {
            "destinationId": station_id,
            "seats": seats,
            "preferExactFit": False,
            "idempotencyKey": key,
        },
    )


def build_lane(
    token: str, prefix: str, n_cars: int, cap: int, block_mod: int
) -> tuple[str, str, set[str], int]:
    """Returns (station_id, friendly_name, blocked_vehicle_ids, initial_total_seats)."""
    sid = f"{prefix}_{uuid.uuid4().hex[:10]}"
    name = f"LS {prefix}"
    create_route(token, sid, name)
    clear_queue(token, sid)
    vids: list[str] = []
    for _ in range(n_cars):
        vids.append(vehicle_create(token, cap))
    entries: list[dict[str, Any]] = []
    for vid in vids:
        entries.append(enqueue(token, sid, name, vid))
    rows = sorted(list_queue(token, sid), key=lambda r: int(r["queuePosition"]))
    blocked_v: set[str] = set()
    for i, row in enumerate(rows, start=1):
        if block_mod > 0 and i % block_mod == 0:
            set_block(token, sid, str(row["id"]), True)
            blocked_v.add(str(row["vehicleId"]))
    total_seats = sum(int(r["availableSeats"]) for r in list_queue(token, sid))
    return sid, name, blocked_v, total_seats


def assert_booking_not_blocked(
    blocked_v: set[str], vid: str, label: str, strict: bool
) -> None:
    if vid in blocked_v:
        raise SystemExit(f"FAIL {label}: booked garage-blocked vehicle {vid}")
    if strict and not vid:
        raise SystemExit(f"FAIL {label}: empty vehicle id")


def run_mass_lane(
    token: str,
    label: str,
    n_cars: int,
    cap: int,
    block_mod: int,
    bookings: int,
    multi_extra: int,
) -> None:
    sid, _name, blocked_v, init_seats = build_lane(token, label, n_cars, cap, block_mod)
    avail0 = bookable_seats(token, sid)
    max_multi_seat = min(4, cap)
    reserve = 8
    # Plan singles first, then cap multi passes by conservative average seat burn (~3).
    bookings = min(bookings, max(0, avail0 - reserve))
    multi_extra = min(
        multi_extra,
        max(0, (avail0 - bookings - reserve) // 3),
    )
    print(
        f"  Lane {label}: dest={sid} cars={n_cars} cap={cap} block_mod={block_mod} "
        f"blocked_vehicles={len(blocked_v)} init_seats≈{init_seats} bookable={avail0} "
        f"plan 1×{bookings} + multi×{multi_extra} (capped to capacity)"
    )
    b = 0
    for i in range(bookings):
        c, bd = book_dest(token, sid, 1, f"{sid}-1-{i}-{random.random()}")
        if c != 201 or not bd.get("success"):
            raise SystemExit(f"FAIL booking {label} #{i} {c} {bd}")
        vid = str(bd["data"]["vehicleId"])
        assert_booking_not_blocked(blocked_v, vid, f"{label} seat {i}", True)
        b += 1
        if (i + 1) % 50 == 0:
            _ = list_queue(token, sid)
    for j in range(multi_extra):
        seats = random.randint(2, min(max_multi_seat, 4))
        c, bd = book_dest(token, sid, seats, f"{sid}-m-{j}-{random.random()}")
        tries = 0
        while c != 201 or not bd.get("success"):
            seats = max(2, seats - 1)
            tries += 1
            if tries > 5:
                raise SystemExit(f"FAIL multi {label} #{j} {c} {bd}")
            c, bd = book_dest(token, sid, seats, f"{sid}-m-{j}-{tries}-{random.random()}")
        vid = str(bd["data"]["vehicleId"])
        assert_booking_not_blocked(blocked_v, vid, f"{label} multi {j}", True)
        b += 1
    print(f"    → completed {b} bookings on {label}")


def run_parallel_destinations(token: str) -> None:
    lanes: list[tuple[str, set[str], int]] = []
    total_bookings = 0
    for d in range(NUM_DESTINATIONS):
        sid, _name, blocked_v, _seats = build_lane(
            token, f"pd{d}", CARS_PER_DEST, CAPACITY, block_mod=6
        )
        lanes.append((sid, blocked_v, 25 + (d % 5)))
    for round_idx in range(max(x[2] for x in lanes)):
        for sid, blocked_v, nbook in lanes:
            if round_idx >= nbook:
                continue
            c, bd = book_dest(token, sid, 1, f"pd-{sid}-{round_idx}-{random.random()}")
            if c != 201 or not bd.get("success"):
                raise SystemExit(f"FAIL parallel round {round_idx} {sid} {c} {bd}")
            assert_booking_not_blocked(blocked_v, str(bd["data"]["vehicleId"]), sid, True)
            total_bookings += 1
    print(f"  Parallel {NUM_DESTINATIONS} destinations × ~25+ bookings ≈ {total_bookings} total")


def run_serving_smoke(token: str) -> None:
    """One lane, same logical scenario as verify_serving_priority_flow (smaller counts)."""
    sid, _name, blocked_v, _ = build_lane(token, "serve_sm", 6, 7, block_mod=0)
    qrows = sorted(list_queue(token, sid), key=lambda r: int(r["queuePosition"]))
    qid2 = str(qrows[1]["id"])
    set_block(token, sid, qid2, True)
    blocked_v.add(str(qrows[1]["vehicleId"]))
    v1 = str(qrows[0]["vehicleId"])
    v3 = str(qrows[2]["vehicleId"])
    v1_avail = int(qrows[0]["availableSeats"])
    for i in range(v1_avail):
        c, bd = book_dest(token, sid, 1, f"sm-fill1-{i}-{random.random()}")
        if c != 201 or not bd.get("success"):
            raise SystemExit(f"serve_sm fill head {c} {bd}")
        got = str(bd["data"]["vehicleId"])
        assert_booking_not_blocked(blocked_v, got, "serve_sm", True)
        if got != v1:
            raise SystemExit(f"serve_sm: booking {i} expected head {v1} got {got}")
    c, bd = book_dest(token, sid, 1, f"sm-gap-{random.random()}")
    if str(bd["data"]["vehicleId"]) != v3:
        raise SystemExit(f"serve_sm: expected C3 after blocked skip got {bd['data']['vehicleId']}")
    set_block(token, sid, qid2, False)
    blocked_v.discard(str(qrows[1]["vehicleId"]))
    c, bd = book_dest(token, sid, 1, f"sm-after-unblock-{random.random()}")
    if str(bd["data"]["vehicleId"]) != v3:
        raise SystemExit("serve_sm: after unblock should still serve C3")
    print("  Serving smoke: PASS")


def main() -> None:
    random.seed(RNG_SEED)
    print(
        f"Large-scale queue booking verify "
        f"(dest={NUM_DESTINATIONS}, cars/dest={CARS_PER_DEST}, cap={CAPACITY}, "
        f"book/dest≈{BOOKINGS_PER_DEST}, multi={MULTI_SEAT_BOOKINGS}, seed={RNG_SEED})"
    )
    token = login()
    time.sleep(2.0)

    t0 = time.time()
    run_mass_lane(
        token,
        "massA",
        n_cars=28,
        cap=CAPACITY,
        block_mod=5,
        bookings=BOOKINGS_PER_DEST,
        multi_extra=MULTI_SEAT_BOOKINGS,
    )
    run_mass_lane(
        token,
        "massB",
        n_cars=30,
        cap=6,
        block_mod=7,
        bookings=110,
        multi_extra=35,
    )
    run_parallel_destinations(token)
    run_serving_smoke(token)
    elapsed = time.time() - t0
    print(f"\nALL LARGE-SCALE CHECKS PASSED in {elapsed:.1f}s")


if __name__ == "__main__":
    main()
