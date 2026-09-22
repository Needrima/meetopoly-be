#!/usr/bin/env python3
"""
Remap africa-1 to classic Monopoly Chance/Chest indices and
set Chance→CHA / Chest→CHE across all worlds.

Classic slots (40-space):
  Chance: 7, 22, 36
  Chest:  2, 17, 33
  Tax income: 4 · Luxury tax: 38
  Rails: 5, 15, 25, 35 · Utils: 12, 28
"""

from __future__ import annotations

import json
from collections import defaultdict
from copy import deepcopy
from pathlib import Path

PATH = Path(__file__).resolve().parents[1] / "locations.json"

CLASSIC: dict[int, tuple[str, str | None]] = {
    0: ("special", "go"),
    1: ("property", None),
    2: ("special", "community_chest"),
    3: ("property", None),
    4: ("special", "tax"),
    5: ("railroad", None),
    6: ("property", None),
    7: ("special", "chance"),
    8: ("property", None),
    9: ("property", None),
    10: ("special", "jail"),
    11: ("property", None),
    12: ("utility", None),
    13: ("property", None),
    14: ("property", None),
    15: ("railroad", None),
    16: ("property", None),
    17: ("special", "community_chest"),
    18: ("property", None),
    19: ("property", None),
    20: ("special", "free_parking"),
    21: ("property", None),
    22: ("special", "chance"),
    23: ("property", None),
    24: ("property", None),
    25: ("railroad", None),
    26: ("property", None),
    27: ("property", None),
    28: ("utility", None),
    29: ("property", None),
    30: ("special", "go_to_jail"),
    31: ("property", None),
    32: ("property", None),
    33: ("special", "community_chest"),
    34: ("property", None),
    35: ("railroad", None),
    36: ("special", "chance"),
    37: ("property", None),
    38: ("special", "tax"),
    39: ("property", None),
}

DROP_SLUGS = {"sao-tome", "windhoek"}


def special_template(
    kind: str,
    st: str | None,
    index: int,
    world: str,
    existing: dict[str, dict],
) -> dict:
    if kind == "railroad":
        slug = {5: "rail-1", 15: "rail-2", 25: "rail-3", 35: "rail-4"}[index]
        return deepcopy(existing[slug])

    if kind == "utility":
        slug = {12: "utility-power", 28: "utility-water"}[index]
        return deepcopy(existing[slug])

    if st == "go":
        return deepcopy(existing["go"])
    if st == "jail":
        return deepcopy(existing["jail"])
    if st == "free_parking":
        return deepcopy(existing["free-parking"])
    if st == "go_to_jail":
        return deepcopy(existing["go-to-jail"])
    if st == "tax" and index == 4:
        return deepcopy(existing["tax-income"])
    if st == "tax" and index == 38:
        return deepcopy(existing["tax-luxury"])

    if st == "chance":
        n = {7: 1, 22: 2, 36: 3}[index]
        slug = "chance" if n == 1 else f"chance-{n}"
        base = deepcopy(existing.get("chance-2") or existing["chance"])
        if n == 1:
            base = deepcopy(existing["chance"])
        base["slug"] = slug
        base["specialType"] = "chance"
        base["kind"] = "special"
        base["name"] = "Chance"
        base["boardCode"] = "CHA"
        base["assets"] = {"icon": "city-icons/generic/question-mark.svg"}
        base["hubId"] = f"hub:{world}:{slug}"
        base["price"] = 0
        base["description"] = "Chance"
        return base

    if st == "community_chest":
        n = {2: 1, 17: 2, 33: 3}[index]
        slug = "community-chest" if n == 1 else f"community-chest-{n}"
        base = deepcopy(existing.get("community-chest-2") or existing["community-chest"])
        if n == 1:
            base = deepcopy(existing["community-chest"])
        base["slug"] = slug
        base["specialType"] = "community_chest"
        base["kind"] = "special"
        base["name"] = "Community Chest"
        base["boardCode"] = "CHE"
        base["assets"] = {"icon": "city-icons/generic/treasure-chest.svg"}
        base["hubId"] = f"hub:{world}:{slug}"
        base["price"] = 0
        base["description"] = "Community Chest"
        return base

    raise KeyError(f"unhandled {kind}/{st} @{index}")


def remap_africa(locs: list[dict]) -> list[dict]:
    africa = [d for d in locs if d["worldId"] == "africa-1"]
    others = [d for d in locs if d["worldId"] != "africa-1"]

    by_slug = {d["slug"]: d for d in africa}
    props = sorted(
        [d for d in africa if d["kind"] == "property" and d["slug"] not in DROP_SLUGS],
        key=lambda d: d["boardIndex"],
    )
    if len(props) != 22:
        raise SystemExit(f"expected 22 properties after drop, got {len(props)}")

    prop_i = 0
    out: list[dict] = []
    for idx in range(40):
        kind, st = CLASSIC[idx]
        if kind == "property":
            doc = deepcopy(props[prop_i])
            prop_i += 1
        else:
            doc = special_template(kind, st, idx, "africa-1", by_slug)

        doc["boardIndex"] = idx
        doc["worldId"] = "africa-1"
        out.append(doc)

    print("africa-1 remapped (Chance/Chest ★):")
    for d in out:
        mark = " ★" if d.get("specialType") in ("chance", "community_chest") else ""
        print(f"  {d['boardIndex']:2} {d.get('boardCode', ''):4} {d['slug']:22} {d['name'][:40]}{mark}")
    print(f"dropped cities: {sorted(DROP_SLUGS)}")
    return others + out


def fix_chance_chest_codes(locs: list[dict]) -> None:
    """Per world: CHA/CHA2/CHA3 and CHE/CHE2/CHE3 (unique). UI still shows CHA/CHE."""
    by_world: dict[str, list[dict]] = defaultdict(list)
    for d in locs:
        by_world[d["worldId"]].append(d)

    for docs in by_world.values():
        for d in docs:
            if d.get("specialType") in ("chance", "community_chest"):
                d["boardCode"] = None

        used = {d.get("boardCode") for d in docs if d.get("boardCode")}

        def take(base: str, i: int) -> str:
            code = base if i == 0 else f"{base}{i + 1}"
            if code in used:
                for other in docs:
                    if other.get("boardCode") == code and other.get("specialType") not in (
                        "chance",
                        "community_chest",
                    ):
                        alt = f"{code[:2]}X"
                        n = 2
                        while alt in used:
                            alt = f"{code[:2]}X{n}"
                            n += 1
                        used.discard(code)
                        other["boardCode"] = alt
                        used.add(alt)
                        break
            n = i + 1
            while True:
                cand = base if n == 1 else f"{base}{n}"
                if cand not in used:
                    return cand
                n += 1

        chances = sorted(
            (d for d in docs if d.get("specialType") == "chance"),
            key=lambda d: d["boardIndex"],
        )
        chests = sorted(
            (d for d in docs if d.get("specialType") == "community_chest"),
            key=lambda d: d["boardIndex"],
        )
        for i, d in enumerate(chances):
            d["boardCode"] = take("CHA", i)
            d["name"] = "Chance"
            used.add(d["boardCode"])
        for i, d in enumerate(chests):
            d["boardCode"] = take("CHE", i)
            d["name"] = "Community Chest"
            used.add(d["boardCode"])



def main() -> None:
    data = json.loads(PATH.read_text())
    data = remap_africa(data)
    fix_chance_chest_codes(data)

    africa = [d for d in data if d["worldId"] == "africa-1"]
    chance_idx = sorted(d["boardIndex"] for d in africa if d.get("specialType") == "chance")
    chest_idx = sorted(
        d["boardIndex"] for d in africa if d.get("specialType") == "community_chest"
    )
    assert chance_idx == [7, 22, 36], chance_idx
    assert chest_idx == [2, 17, 33], chest_idx
    assert len(africa) == 40
    assert len([d for d in africa if d["kind"] == "property"]) == 22

    # right-side mid tiles sanity
    right = {d["boardIndex"]: d for d in africa}
    assert right[33]["specialType"] == "community_chest"
    assert right[36]["specialType"] == "chance"
    assert right[38]["specialType"] == "tax"

    PATH.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
    print(f"wrote {PATH} ({len(data)} docs)")


if __name__ == "__main__":
    main()
