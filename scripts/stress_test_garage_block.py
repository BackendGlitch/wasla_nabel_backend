#!/usr/bin/env python3
"""
Exercise garage block/unblock vs booking across multiple destinations.
Requires: jq-style nothing; stdlib urllib only. Backend services listening per env vars.

Usage (from repo or any cwd):
  python3 scripts/stress_test_garage_block.py

Env (optional):
  WASLA_AUTH_URL   default http://localhost:8001/api/v1
  WASLA_QUEUE_URL  default http://localhost:8002/api/v1
  WASLA_BOOK_URL   default http://localhost:8003/api/v1
"""
from __future__ import annotations

import json
import os
import random
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
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


def require(ok: bool, name: str, detail: str) -> None:
    tag = "PASS" if ok else "FAIL"
    print(f"  [{tag}] {name}: {detail}")
    if not ok:
        raise SystemExit(1)


def login_cin(token_cin: str) -> str:
    code, data = http_json("POST", f"{AUTH}/auth/login", body={"cin": token_cin})
    if code != 200 or not data.get("success"):
        raise SystemExit(f"login failed {code}: {data}")
    tok = (data.get("data") or {}).get("token")
    if not tok:
        raise SystemExit(f"no token: {data}")
    return tok


def create_route(token: str, station_id: str, name: str, price: float = 7.5) -> None:
    code, data = http_json(
        "POST",
        f"{QUEUE}/routes",
        token,
        {
            "stationId": station_id,
            "stationName": name,
            "basePrice": price,
        },
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"create route {station_id} failed {code}: {data}")


def clear_queue(token: str, station_id: str) -> None:
    http_json("DELETE", f"{QUEUE}/queue/{station_id}/clear", token)


def vehicle_create(token: str, plate_suffix: str) -> str:
    # NN TUN NNNN pattern
    a = random.randint(10, 99)
    code, data = http_json(
        "POST",
        f"{QUEUE}/vehicles",
        token,
        {"licensePlate": f"{a} TUN {plate_suffix}", "capacity": 9},
    )
    if code != 201 or not data.get("success"):
        raise SystemExit(f"vehicle failed {plate_suffix} {code}: {data}")
    return (data.get("data") or {})["id"]


def enqueue(
    token: str, station_id: str, name: str, vehicle_id: str
) -> dict[str, Any]:
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
        raise SystemExit(f"enqueue failed {vehicle_id}->{station_id}: {code} {data}")
    return (data.get("data") or {})["queueEntry"]


def list_queue(token: str, station_id: str, exclude_blocked: bool) -> list[dict]:
    qs = "?excludeGarageBlocked=true" if exclude_blocked else ""
    code, data = http_json("GET", f"{QUEUE}/queue/{station_id}{qs}", token)
    if code != 200 or not data.get("success"):
        raise SystemExit(f"list queue failed: {code} {data}")
    return data["data"] or []


def q_sorted(token: str, station_id: str) -> list[dict]:
    rows = list_queue(token, station_id, exclude_blocked=False)
    return sorted(rows, key=lambda r: int(r["queuePosition"]))


def set_block(token: str, station_id: str, entry_id: str, blocked: bool) -> None:
    code, data = http_json(
        "PUT",
        f"{QUEUE}/queue/{station_id}/entry/{entry_id}/garage-block",
        token,
        {"blocked": blocked},
    )
    if code != 200 or not data.get("success"):
        raise SystemExit(f"garage-block failed: {blocked} {code}: {data}")


def booking_by_dest(token: str, station_id: str, seats: int, prefer_exact: bool) -> tuple[int, dict]:
    return http_json(
        "POST",
        f"{BOOK}/bookings",
        token,
        {
            "destinationId": station_id,
            "seats": seats,
            "preferExactFit": prefer_exact,
            "idempotencyKey": f"py-{station_id}-{seats}-{random.random()}",
        },
    )


def booking_by_entry(token: str, entry_id: str, seats: int) -> tuple[int, dict]:
    return http_json(
        "POST",
        f"{BOOK}/bookings/by-queue-entry",
        token,
        {
            "queueEntryId": entry_id,
            "seats": seats,
            "idempotencyKey": f"qb-{entry_id}-{random.random()}",
        },
    )


def update_entry_avail(token: str, station_id: str, entry_id: str, seats: int) -> None:
    code, data = http_json(
        "PUT",
        f"{QUEUE}/queue/{station_id}/entry/{entry_id}",
        token,
        {"availableSeats": seats},
    )
    if code != 200 or not data.get("success"):
        raise SystemExit(f"update avail failed {code}: {data}")


@dataclass
class Dest:
    station_id: str
    friendly: str


def main() -> None:
    suf = "".join(random.choices("abcdefghijklmnopqrstuvwxyz", k=10))
    d_big = Dest(f"stress_{suf}_big", "Stress Big Queue")
    d_mid = Dest(f"stress_{suf}_mid", "Stress Mid Queue")
    d_sml = Dest(f"stress_{suf}_sml", "Stress Small Queue")
    destinations = [d_big, d_mid, d_sml]

    counts = {"big": 10, "mid": 6, "sml": 4}
    print("Stress test garage block vs booking — suffix", suf)
    print("Login supervisor CIN…")
    token = login_cin("12345678")
    print("JWT ok (wait for Redis session)…")
    time.sleep(1.8)

    for d in destinations:
        print(f"Route + clear {d.station_id}")
        create_route(token, d.station_id, d.friendly)
        clear_queue(token, d.station_id)

    plates = [f"{8000+i:04d}" for i in range(20)]
    vids_batch: dict[str, list[str]] = {d_big.station_id: [], d_mid.station_id: [], d_sml.station_id: []}
    i = 0
    for _ in range(counts["big"]):
        vids_batch[d_big.station_id].append(vehicle_create(token, plates[i]))
        i += 1
    for _ in range(counts["mid"]):
        vids_batch[d_mid.station_id].append(vehicle_create(token, plates[i]))
        i += 1
    for _ in range(counts["sml"]):
        vids_batch[d_sml.station_id].append(vehicle_create(token, plates[i]))
        i += 1

    for d in destinations:
        for vid in vids_batch[d.station_id]:
            enqueue(token, d.station_id, d.friendly, vid)

    def at_pos(rows: list[dict], position: int) -> dict:
        for r in rows:
            if int(r["queuePosition"]) == position:
                return r
        raise KeyError(position)

    # --- Scenario 1: block head of big queue ---
    qb = q_sorted(token, d_big.station_id)
    entry1 = qb[0]  # pos 1
    require(int(entry1["queuePosition"]) == 1, "setup big pos 1", str(entry1["queuePosition"]))
    set_block(token, d_big.station_id, entry1["id"], True)
    ex = len(list_queue(token, d_big.station_id, exclude_blocked=True))
    require(ex == 9, "exclude count after block head", str(ex))

    code, bd = booking_by_dest(token, d_big.station_id, 1, False)
    second_vid = qb[1]["vehicleId"]
    require(
        code == 201 and bd.get("success") and bd["data"]["vehicleId"] == second_vid,
        "booking skips blocked head",
        f"code={code} got={bd.get('data',{}).get('vehicleId')} want={second_vid}",
    )

    code, bx = booking_by_entry(token, entry1["id"], 1)
    # Booking handler wraps repo error ("blocked in garage") as InternalServerError generic message.
    require(
        code >= 400 and bx.get("success") is False,
        "by-queue-entry on blocked rejects",
        f"code={code} body={bx}",
    )

    set_block(token, d_big.station_id, entry1["id"], False)
    qb = q_sorted(token, d_big.station_id)

    # --- Scenario 2: block SECOND only ---
    e2 = at_pos(qb, 2)
    set_block(token, d_big.station_id, e2["id"], True)
    third_vid = at_pos(qb, 3)["vehicleId"]
    head_e = at_pos(qb, 1)
    code, bd = booking_by_dest(token, d_big.station_id, 1, False)
    require(
        code == 201 and bd["data"]["vehicleId"] == head_e["vehicleId"],
        "block mid (pos2) bookings take pos1",
        f'{bd.get("data",{}).get("vehicleId")} vs {head_e["vehicleId"]}',
    )
    # Drain pos1 so the next single-seat booking must skip blocked pos2.
    update_entry_avail(token, d_big.station_id, head_e["id"], 0)
    code, bd = booking_by_dest(token, d_big.station_id, 1, False)
    require(
        code == 201 and bd["data"]["vehicleId"] == third_vid,
        "with pos1 empty skips blocked pos2 -> pos3",
        f'{bd["data"]["vehicleId"]}',
    )
    set_block(token, d_big.station_id, e2["id"], False)
    # Restore pos1 capacity for later scenarios on d_big.
    update_entry_avail(token, d_big.station_id, head_e["id"], int(head_e["totalSeats"]))

    qb = q_sorted(token, d_big.station_id)
    # --- Scenario 3: block LAST (big has 10) ---
    elo = qb[-1]
    require(int(elo["queuePosition"]) == 10, "big last position", elo["queuePosition"])
    set_block(token, d_big.station_id, elo["id"], True)
    first_vid = qb[0]["vehicleId"]
    code, bd = booking_by_dest(token, d_big.station_id, 1, False)
    require(code == 201 and bd["data"]["vehicleId"] == first_vid, "block last ignores tail", bd.get("message"))
    set_block(token, d_big.station_id, elo["id"], False)

    # --- Scenario 4: block ALL of small queue -> no vehicle ---
    qs = q_sorted(token, d_sml.station_id)
    require(len(qs) == 4, "small queue depth", len(qs))
    for row in qs:
        set_block(token, d_sml.station_id, row["id"], True)
    require(len(list_queue(token, d_sml.station_id, True)) == 0, "all small blocked excludes all", "...")
    code, bd = booking_by_dest(token, d_sml.station_id, 1, False)
    err_blob = f"{bd.get('message','')} {bd.get('error','')}".lower()
    require(
        code >= 400 and "no vehicle" in err_blob,
        "all blocked rejects destination booking",
        f"{code} {bd}",
    )
    for row in qs:
        set_block(token, d_sml.station_id, row["id"], False)

    # --- Scenario 5: mid queue isolation ---
    mids = q_sorted(token, d_mid.station_id)
    mid_pick = mids[2]["vehicleId"]
    mid_entry = mids[2]["id"]
    set_block(token, d_mid.station_id, mid_entry, True)
    code, bd = booking_by_dest(token, d_mid.station_id, 1, False)
    require(code == 201 and bd["data"]["vehicleId"] == mids[0]["vehicleId"], "mid dest head free", "...")
    set_block(token, d_mid.station_id, mid_entry, False)

    # --- Scenario 6: unblock then booking hits previously blocked ---
    qb = q_sorted(token, d_big.station_id)
    em = qb[5]  # pos 6 = "middle-ish"
    set_block(token, d_big.station_id, em["id"], True)
    fv = qb[0]["vehicleId"]
    code, bd = booking_by_dest(token, d_big.station_id, 1, False)
    require(code == 201 and bd["data"]["vehicleId"] == fv, "middle blocked booking still hits pos1", "")
    code, bn = booking_by_entry(token, em["id"], 1)
    require(code >= 400 and bn.get("success") is False, "blocked middle rejects direct booking", str(bn))
    set_block(token, d_big.station_id, em["id"], False)
    # direct booking ok after unblock + still has seats
    code, bo = booking_by_entry(token, em["id"], 1)
    require(code == 201 and bo.get("success"), "after unblock entry books", str(bo.get("message")))

    # --- Scenario 7: preferExactFit skips blocked candidate (two full rows equal capacity) ---
    qb = q_sorted(token, d_big.station_id)
    e_head = qb[0]
    e_next = qb[1]
    cap = int(e_next["totalSeats"])
    update_entry_avail(token, d_big.station_id, e_next["id"], cap)
    update_entry_avail(token, d_big.station_id, e_head["id"], cap)
    set_block(token, d_big.station_id, e_head["id"], True)
    code, bd = booking_by_dest(token, d_big.station_id, cap, True)
    require(
        code == 201 and bd["data"]["vehicleId"] == e_next["vehicleId"],
        "preferExactFit skips blocked head when both seat counts match",
        f'{bd["data"]["vehicleId"]}',
    )
    set_block(token, d_big.station_id, e_head["id"], False)

    # --- Scenario 8: full-car booking skips blocked middle (deterministic on d_sml, 4 cars) ---
    for row in q_sorted(token, d_sml.station_id):
        set_block(token, d_sml.station_id, row["id"], False)
    qs = q_sorted(token, d_sml.station_id)
    seats_full = int(qs[0]["totalSeats"])
    mid = at_pos(qs, 2)
    set_block(token, d_sml.station_id, mid["id"], True)
    v_pos1 = at_pos(qs, 1)["vehicleId"]
    v_pos3 = at_pos(qs, 3)["vehicleId"]
    code, bk1 = booking_by_dest(token, d_sml.station_id, seats_full, False)
    require(code == 201 and bk1["data"]["vehicleId"] == v_pos1, "sml first booking pos1 only", bk1.get("message", ""))
    code, bk2 = booking_by_dest(token, d_sml.station_id, seats_full, False)
    require(code == 201 and bk2["data"]["vehicleId"] == v_pos3, "sml skips blocked pos2→pos3", bk2["data"]["vehicleId"])
    set_block(token, d_sml.station_id, mid["id"], False)

    print("")
    print("All stress scenarios PASSED.")

    # Optional teardown of test routes impossible without delete-route + queue clear; queues may retain state.
    print(
        "(Left test queues/routes/v DB rows; rerun init or delete routes manually if you want a clean slate.)"
    )


if __name__ == "__main__":
    main()
