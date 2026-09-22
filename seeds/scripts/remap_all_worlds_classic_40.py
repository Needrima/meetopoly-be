#!/usr/bin/env python3
"""
Remap EVERY world to classic Monopoly 40 spaces.

Template (boardIndex):
  Chance 7, 22, 36 · Chest 2, 17, 33 · Tax 4, 38
  Rails 5, 15, 25, 35 · Utils 12, 28
  Corners 0 GO, 10 Jail, 20 Free Parking/Layover, 30 Go to Jail
  22 properties

Trim: keep first 22 properties by boardIndex (extras parked only if ≥15 for a future -2; we do not create -2 here).
Pad: if <22 properties, cycle-repeat from that world's cities with unique slug suffixes.
"""

from __future__ import annotations

import json
import re
from collections import Counter, defaultdict
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

RAIL_SLOTS = [5, 15, 25, 35]
UTIL_SLOTS = [12, 28]  # power, water


def find_by_special(docs: list[dict], st: str) -> dict | None:
    for d in docs:
        if d.get("specialType") == st:
            return d
    return None


def find_slug(docs: list[dict], slug: str) -> dict | None:
    for d in docs:
        if d.get("slug") == slug:
            return d
    return None


def ensure_special(
    docs: list[dict],
    world: str,
    *,
    slug: str,
    special_type: str,
    name: str,
    board_code: str,
    icon: str,
    donor: dict | None = None,
) -> dict:
    existing = find_slug(docs, slug) or find_by_special(docs, special_type)
    if existing and existing.get("specialType") == special_type:
        base = deepcopy(existing)
    elif donor:
        base = deepcopy(donor)
    else:
        base = {
            "worldId": world,
            "kind": "special",
            "price": 0,
            "map": {"x": 0, "z": 0, "scale": 1},
            "enterRadius": 1.2,
            "hubId": f"hub:{world}:{slug}",
            "assets": {"icon": icon},
        }
    base["worldId"] = world
    base["slug"] = slug
    base["kind"] = "special"
    base["specialType"] = special_type
    base["name"] = name
    base["boardCode"] = board_code
    base["assets"] = {"icon": icon}
    base["hubId"] = f"hub:{world}:{slug}"
    base["price"] = 0
    base["description"] = name
    if special_type == "free_parking":
        base["name"] = "Layover"
        base["description"] = "Layover"
        base["aboutShort"] = "Layover — free rest stop"
    return base


def ensure_rail(docs: list[dict], world: str, n: int, donor_pool: list[dict]) -> dict:
    slug = f"rail-{n}"
    existing = find_slug(docs, slug)
    if existing:
        return deepcopy(existing)
    # reuse any railroad as template
    template = donor_pool[min(n - 1, len(donor_pool) - 1)] if donor_pool else None
    if not template:
        template = {
            "kind": "railroad",
            "price": 200,
            "map": {"x": 0, "z": 0, "scale": 1},
            "enterRadius": 1.5,
            "assets": {"icon": "city-icons/generic/plane-tilt.svg"},
        }
    base = deepcopy(template)
    base["worldId"] = world
    base["slug"] = slug
    base["kind"] = "railroad"
    base["name"] = base.get("name") or f"Air Hub {n}"
    if not base.get("boardCode"):
        base["boardCode"] = f"R{n}"
    base["assets"] = {"icon": "city-icons/generic/plane-tilt.svg"}
    base["hubId"] = f"hub:{world}:{slug}"
    base.pop("specialType", None)
    return base


def ensure_util(docs: list[dict], world: str, which: str, donor_pool: list[dict]) -> dict:
    slug = f"utility-{which}"
    existing = find_slug(docs, slug)
    if existing:
        return deepcopy(existing)
    template = None
    for d in donor_pool:
        if which in d.get("slug", "") or which in d.get("name", "").lower():
            template = d
            break
    if not template and donor_pool:
        template = donor_pool[0]
    if not template:
        template = {
            "kind": "utility",
            "price": 150,
            "map": {"x": 0, "z": 0, "scale": 1},
            "enterRadius": 1.5,
        }
    base = deepcopy(template)
    base["worldId"] = world
    base["slug"] = slug
    base["kind"] = "utility"
    if which == "power":
        base["name"] = base.get("name") if "power" in base.get("name", "").lower() or "electric" in base.get("name", "").lower() else f"{world} Power Grid"
        base["boardCode"] = "ELC"
        base["assets"] = {"icon": "city-icons/generic/bolt.svg"}
    else:
        base["name"] = base.get("name") if "water" in base.get("name", "").lower() else f"{world} Water Works"
        base["boardCode"] = "WTR"
        base["assets"] = {"icon": "city-icons/generic/droplet.svg"}
    base["hubId"] = f"hub:{world}:{slug}"
    base.pop("specialType", None)
    return base


def pick_properties(docs: list[dict]) -> tuple[list[dict], list[dict]]:
    props = sorted(
        [d for d in docs if d["kind"] == "property"],
        key=lambda d: d["boardIndex"],
    )
    if len(props) >= 22:
        return [deepcopy(p) for p in props[:22]], [deepcopy(p) for p in props[22:]]
    # pad by cycling
    out = [deepcopy(p) for p in props]
    i = 0
    while len(out) < 22:
        src = props[i % len(props)]
        pad = deepcopy(src)
        n = (len(out) // max(len(props), 1)) + 1
        pad["slug"] = f"{src['slug']}-r{n}"
        pad["hubId"] = f"hub:{src['worldId']}:{pad['slug']}"
        # unique boardCode later
        out.append(pad)
        i += 1
    return out, []


def unique_board_codes(docs: list[dict]) -> None:
    # Clear chance/chest first
    for d in docs:
        if d.get("specialType") in ("chance", "community_chest"):
            d["boardCode"] = None

    used: set[str] = set()
    for d in docs:
        c = d.get("boardCode")
        if c:
            used.add(c)

    def take(preferred: str) -> str:
        if preferred not in used:
            used.add(preferred)
            return preferred
        base = re.sub(r"\d+$", "", preferred) or preferred
        n = 2
        while f"{base}{n}" in used:
            n += 1
        code = f"{base}{n}"
        used.add(code)
        return code

    chances = sorted(
        (d for d in docs if d.get("specialType") == "chance"),
        key=lambda d: d["boardIndex"],
    )
    chests = sorted(
        (d for d in docs if d.get("specialType") == "community_chest"),
        key=lambda d: d["boardIndex"],
    )
    for i, d in enumerate(chances):
        d["boardCode"] = take("CHA" if i == 0 else f"CHA{i + 1}")
    for i, d in enumerate(chests):
        d["boardCode"] = take("CHE" if i == 0 else f"CHE{i + 1}")

    # ensure every doc has a code and uniqueness
    for d in docs:
        code = d.get("boardCode") or "X"
        if code in used and sum(1 for x in docs if x.get("boardCode") == code) > 1:
            # will fix below
            pass
        elif not d.get("boardCode"):
            d["boardCode"] = take("X")

    # final uniqueness pass
    seen: set[str] = set()
    for d in docs:
        code = d.get("boardCode") or "X"
        if code in seen:
            d["boardCode"] = take(code)
        else:
            seen.add(code)
            used.add(code)


def remap_world(world_id: str, docs: list[dict], africa_donor: list[dict]) -> list[dict]:
    props, leftovers = pick_properties(docs)
    if leftovers and len(leftovers) >= 15:
        print(f"  note: {world_id} has {len(leftovers)} leftover cities (≥15) — not creating -2 in this pass")
    elif leftovers:
        print(f"  note: {world_id} parked {len(leftovers)} leftover cities (<15, no -2)")

    rails_existing = sorted(
        [d for d in docs if d["kind"] == "railroad"],
        key=lambda d: d["boardIndex"],
    )
    utils_existing = [d for d in docs if d["kind"] == "utility"]

    # donors from africa for missing specials
    africa_by_slug = {d["slug"]: d for d in africa_donor}

    go = ensure_special(
        docs, world_id, slug="go", special_type="go", name="GO", board_code="GO",
        icon="city-icons/generic/arrow-narrow-right.svg",
        donor=find_by_special(docs, "go") or africa_by_slug.get("go"),
    )
    jail = ensure_special(
        docs, world_id, slug="jail", special_type="jail", name="Jail — Just Visiting",
        board_code="JAL", icon="city-icons/generic/prison.svg",
        donor=find_by_special(docs, "jail") or africa_by_slug.get("jail"),
    )
    parking = ensure_special(
        docs, world_id, slug="free-parking", special_type="free_parking", name="Layover",
        board_code="P", icon="city-icons/generic/parking.svg",
        donor=find_by_special(docs, "free_parking") or africa_by_slug.get("free-parking"),
    )
    goto = ensure_special(
        docs, world_id, slug="go-to-jail", special_type="go_to_jail", name="Go to Jail",
        board_code="GTJ", icon="city-icons/generic/prison.svg",
        donor=find_by_special(docs, "go_to_jail") or africa_by_slug.get("go-to-jail"),
    )
    tax_in = ensure_special(
        docs, world_id, slug="tax-income", special_type="tax", name="Income Tax",
        board_code="TAX", icon="city-icons/generic/tax.svg",
        donor=find_slug(docs, "tax-income") or find_by_special(docs, "tax") or africa_by_slug.get("tax-income"),
    )
    tax_lux = ensure_special(
        docs, world_id, slug="tax-luxury", special_type="tax", name="Luxury Tax",
        board_code="LTX", icon="city-icons/generic/tax.svg",
        donor=find_slug(docs, "tax-luxury") or africa_by_slug.get("tax-luxury") or tax_in,
    )

    chances = []
    for n, idx in enumerate([7, 22, 36], start=1):
        slug = "chance" if n == 1 else f"chance-{n}"
        chances.append(
            ensure_special(
                docs, world_id, slug=slug, special_type="chance", name="Chance",
                board_code="CHA", icon="city-icons/generic/question-mark.svg",
                donor=find_by_special(docs, "chance") or africa_by_slug.get("chance"),
            )
        )

    chests = []
    for n, idx in enumerate([2, 17, 33], start=1):
        slug = "community-chest" if n == 1 else f"community-chest-{n}"
        chests.append(
            ensure_special(
                docs, world_id, slug=slug, special_type="community_chest", name="Community Chest",
                board_code="CHE", icon="city-icons/generic/treasure-chest.svg",
                donor=find_by_special(docs, "community_chest") or africa_by_slug.get("community-chest"),
            )
        )

    rails = [ensure_rail(docs, world_id, n, rails_existing) for n in range(1, 5)]
    util_power = ensure_util(docs, world_id, "power", utils_existing)
    util_water = ensure_util(docs, world_id, "water", utils_existing)

    prop_i = 0
    chance_i = 0
    chest_i = 0
    rail_i = 0
    out: list[dict] = []

    for idx in range(40):
        kind, st = CLASSIC[idx]
        if kind == "property":
            doc = props[prop_i]
            prop_i += 1
        elif kind == "railroad":
            doc = rails[rail_i]
            rail_i += 1
        elif kind == "utility":
            doc = util_power if idx == 12 else util_water
        elif st == "go":
            doc = go
        elif st == "jail":
            doc = jail
        elif st == "free_parking":
            doc = parking
        elif st == "go_to_jail":
            doc = goto
        elif st == "tax" and idx == 4:
            doc = tax_in
        elif st == "tax" and idx == 38:
            doc = tax_lux
        elif st == "chance":
            doc = chances[chance_i]
            chance_i += 1
        elif st == "community_chest":
            doc = chests[chest_i]
            chest_i += 1
        else:
            raise RuntimeError(f"unhandled {kind}/{st}")

        doc = deepcopy(doc)
        doc["worldId"] = world_id
        doc["boardIndex"] = idx
        out.append(doc)

    unique_board_codes(out)

    # validate
    assert len(out) == 40
    assert len({d["slug"] for d in out}) == 40, Counter(d["slug"] for d in out)
    codes = [d["boardCode"] for d in out]
    assert len(codes) == len(set(codes)), Counter(codes)
    assert sorted(d["boardIndex"] for d in out if d.get("specialType") == "chance") == [7, 22, 36]
    assert sorted(d["boardIndex"] for d in out if d.get("specialType") == "community_chest") == [2, 17, 33]

    return out


def main() -> None:
    data = json.loads(PATH.read_text())
    by_world: dict[str, list[dict]] = defaultdict(list)
    for d in data:
        by_world[d["worldId"]].append(d)

    africa = by_world["africa-1"]
    rebuilt: list[dict] = []
    for world_id in sorted(by_world.keys()):
        print(f"remapping {world_id}…")
        rebuilt.extend(remap_world(world_id, by_world[world_id], africa))

    # summary
    print("\nSummary:")
    worlds = defaultdict(list)
    for d in rebuilt:
        worlds[d["worldId"]].append(d)
    for w in sorted(worlds):
        print(f"  {w}: {len(worlds[w])} spaces, props={sum(1 for d in worlds[w] if d['kind']=='property')}")

    PATH.write_text(json.dumps(rebuilt, indent=2, ensure_ascii=False) + "\n")
    print(f"\nwrote {PATH} ({len(rebuilt)} docs)")


if __name__ == "__main__":
    main()
