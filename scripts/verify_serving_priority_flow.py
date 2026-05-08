#!/usr/bin/env python3
"""
Deep E2E: serving row + unblock priority + compaction (migration 027+).

Scenario (4 cars, same destination):
  C1 fills first while C2 is garage-blocked → bookings skip to C3.
  Unblock C2 → next bookings must STILL serve C3 (serving_focus) until C3 is full.
  After C3 full → next booking must land on C2 (prioritize_after_blocked_unblock) before C4.

Env: WASLA_* URLs like stress_test_garage_block.py (defaults localhost).

Usage:
  python3 scripts/verify_serving_priority_flow.py
"""
from __future__ import annotations

import json
import os
import random
import time
import urllib.error
import urllib.request
from typing import Any, Optional

AUTH = os.environ.get("WASLA_AUTH_URL", "http://localhost:8001/api/v1").rstrip("/")
QUEUE = os.environ.get("WASLA_QUEUE_URL", "http://localhost:8002/api/v1").rstrip("/")
BOOK = os.environ.get("WASLA_BOOK_URL", "http://localhost:8003/api/v1").rstrip("/")


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
        with urllib.request.urlopen(req, timeout=120) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw.strip() else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"message": raw, "_raw": True}


def login_cin(token_cin: str) -> str:
    code, data = http_json("POST", f"{AUTH}/auth/login", body={"cin": token_cin})
    if code != 200 or not data.get("success"):
        raise SystemExit(f"login failed {code}: {data}")
    return (data.get("data") or {})["token"]


def http_ok(code: int) -> bool:
    return 200 <= code < 300


def create_route(token: str, station_id: str, name: str) -> None:
    code, data = http_json(
        "POST",
        f"{QUEUE}/routes",
        token,
        {"stationId": station_id, "stationName": name, "basePrice": 8.0},
    )
    if not http_ok(code) or not data.get("success"):
        raise SystemExit(f"route {station_id} {code}: {data}")


def clear_queue(token: str, station_id: str) -> None:
    http_json("DELETE", f"{QUEUE}/queue/{station_id}/clear", token)


def vehicle_create(token: str, plate_suffix: str, cap: int) -> str:
    a = random.randint(21, 98)
    code, data = http_json(
        "POST",
        f"{QUEUE}/vehicles",
        token,
        {"licensePlate": f"{a} TUN {plate_suffix}", "capacity": cap},
    )
    if not http_ok(code) or not data.get("success"):
        raise SystemExit(f"vehicle {plate_suffix}: {code} {data}")
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
    if not http_ok(code) or not data.get("success"):
        raise SystemExit(f"enqueue {vehicle_id}: {code} {data}")
    return (data.get("data") or {})["queueEntry"]


def set_block(token: str, station_id: str, entry_id: str, blocked: bool) -> None:
    code, data = http_json(
        "PUT",
        f"{QUEUE}/queue/{station_id}/entry/{entry_id}/garage-block",
        token,
        {"blocked": blocked},
    )
    if code != 200 or not data.get("success"):
        raise SystemExit(f"garage-block {blocked} {code}: {data}")


def book_dest(token: str, destination_id: str, seats: int, key: str) -> tuple[int, dict[str, Any]]:
    return http_json(
        "POST",
        f"{BOOK}/bookings",
        token,
        {
            "destinationId": destination_id,
            "seats": seats,
            "preferExactFit": False,
            "idempotencyKey": key,
        },
    )


def main() -> None:
    print("Serving + priority queue flow verification")
    suf = "".join(random.choices("abcdefgh0123456789", k=8))
    sid = f"srve_{suf}"
    friendly = "Serve priority test route"
    # Need surplus seats on C3 so after unblock we can prove multiple successive serves there.
    capacity = 8

    token = login_cin("12345678")
    time.sleep(2.0)
    print(f"station_id={sid}")
    create_route(token, sid, friendly)
    clear_queue(token, sid)

    # Tunisian NN TUN NNNN — four-digit suffix uniquely per vehicle
    pid = random.randint(6000, 8990)
    vid1 = vehicle_create(token, f"{pid:04d}", capacity)
    vid2 = vehicle_create(token, f"{pid+1:04d}", capacity)
    vid3 = vehicle_create(token, f"{pid+2:04d}", capacity)
    vid4 = vehicle_create(token, f"{pid+3:04d}", capacity)

    e1 = enqueue(token, sid, friendly, vid1)
    e2 = enqueue(token, sid, friendly, vid2)
    e3 = enqueue(token, sid, friendly, vid3)
    e4 = enqueue(token, sid, friendly, vid4)
    qid1, qid2, qid3, qid4 = str(e1["id"]), str(e2["id"]), str(e3["id"]), str(e4["id"])
    print("vehicles queued C1..C4 (position order at enqueue)", vid1[:8], "...")

    set_block(token, sid, qid2, True)

    def one_book(expect_vid: Optional[str], label: str, reject_vid: Optional[str] = None) -> str:
        c, bd = book_dest(token, sid, 1, f"v-{label}-{random.random()}")
        if c != 201 or not bd.get("success"):
            raise SystemExit(f"BOOK FAIL {label} {c} {bd}")
        gv = str(bd["data"]["vehicleId"])
        ok = True
        if expect_vid is not None and gv != expect_vid:
            ok = False
        if reject_vid is not None and gv == reject_vid:
            ok = False
        tag = "PASS" if ok else "FAIL"
        print(f"  [{tag}] {label}: booked vehicle={gv[:8]}... expect={expect_vid[:8]+'...' if expect_vid else '?'} reject={reject_vid[:8]+'...' if reject_vid else '-'}")
        if not ok:
            raise SystemExit(1)
        return gv

    # Fill car 1 (blocked C2 skipped only after C1 exhausted)
    for i in range(capacity):
        one_book(vid1, f"fill_C1_step_{i+1}")
    print("→ C1 should be READY/full; C2 blocked; compaction may have reshuffled qp")

    # First seat after C1 must go to C3 (not blocked C4-first by qp if C4 pos lower? order by qp with serving none - smallest qp eligible non-blocked: after compaction whichever is front among C3,C4 - user story expects skipping C2)
    gv = one_book(None, "first_after_C1_must_be_C3_not_C2", reject_vid=vid2)
    if gv != vid3:
        # allow C4 only if qp placed C4 ahead of C3 without compaction bug — story wants "skip blocked to next"
        if gv == vid4:
            raise SystemExit("FAIL: first open seat went to C4 instead of expected C3 (check queue order)")
        raise SystemExit(f"FAIL unexpected vehicle after C1: {gv}")

    one_book(vid3, "anchor_serving_second_seat_same_car")

    set_block(token, sid, qid2, False)
    print("→ unblocked C2; next bookings must KEEP serving C3")

    # Exhaust remaining seats on C3 after two pre-unblock bookings (first gap + anchor).
    for i in range(capacity - 2):
        one_book(vid3, f"after_unblock_still_C3_{i+1}")

    # C3 full; serving cleared → C2 prioritized over C4
    gv = one_book(None, "after_C3_full_next_vehicle")
    if gv != vid2:
        raise SystemExit(
            f"FAIL: expected prioritize C2 after C3 full, got vid={gv} "
            "(migration 027 + booking binary running?)"
        )
    print("→ C2 received first booking after C3 full (prioritize_after_blocked_unblock) ✓")

    # One more arbitrary booking — priority flag cleared after first booking on C2; should still seat C2 (has room) before starving C4
    one_book(vid2, "second_on_C2_uses_remainder")

    print("\nALL CHECKS PASSED for serving + unblock + priority scenario.")


if __name__ == "__main__":
    main()
