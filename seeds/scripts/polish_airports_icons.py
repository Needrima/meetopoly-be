#!/usr/bin/env python3
"""Polish seed: prison/tax icons, real airports + codes for all worlds."""

from __future__ import annotations

import json
import re
import unicodedata
from collections import Counter, defaultdict
from pathlib import Path

PATH = Path(__file__).resolve().parents[1] / "locations.json"

PRISON = "city-icons/generic/prison.svg"
TAX = "city-icons/generic/tax.svg"
HANDCUFFS = "city-icons/generic/handcuffs.svg"
PLANE = "city-icons/generic/plane-tilt.svg"

# rail slug → (name, boardCode) per world
AIRPORTS: dict[str, dict[str, tuple[str, str]]] = {
    "africa-1": {
        "rail-1": ("Cairo International Airport", "CAI"),
        "rail-2": ("Jomo Kenyatta International Airport", "NBO"),
        "rail-3": ("O.R. Tambo International Airport", "JNB"),
        "rail-4": ("Murtala Muhammed International Airport", "MM"),
    },
    "europe-1": {
        "rail-1": ("Heathrow Airport", "LHR"),
        "rail-2": ("Charles de Gaulle Airport", "CDG"),
        "rail-3": ("Adolfo Suárez Madrid-Barajas Airport", "MAD"),
        "rail-4": ("Amsterdam Schiphol Airport", "AMS"),
    },
    "europe-2": {
        "rail-1": ("Frankfurt Airport", "FRA"),
        "rail-2": ("Munich Airport", "MUC"),
        "rail-3": ("Zurich Airport", "ZRH"),
        "rail-4": ("Vienna International Airport", "VIE"),
    },
    "europe-3": {
        "rail-1": ("Leonardo da Vinci–Fiumicino Airport", "FCO"),
        "rail-2": ("Milan Malpensa Airport", "MXP"),
        "rail-3": ("Athens International Airport", "ATH"),
        "rail-4": ("Lisbon Humberto Delgado Airport", "LIS"),
    },
    "europe-4": {
        "rail-1": ("Stockholm Arlanda Airport", "ARN"),
        "rail-2": ("Oslo Gardermoen Airport", "OSL"),
        "rail-3": ("Copenhagen Airport", "CPH"),
        "rail-4": ("Helsinki Airport", "HEL"),
    },
    "europe-5": {
        "rail-1": ("Warsaw Chopin Airport", "WAW"),
        "rail-2": ("Václav Havel Airport Prague", "PRG"),
        "rail-3": ("Budapest Ferenc Liszt Airport", "BUD"),
        "rail-4": ("Bucharest Henri Coandă Airport", "OTP"),
    },
    "asia-1": {
        "rail-1": ("Tokyo Narita International Airport", "NRT"),
        "rail-2": ("Beijing Capital International Airport", "PEK"),
        "rail-4": ("Singapore Changi Airport", "SIN"),
    },
    "asia-2": {
        "rail-1": ("Seoul Incheon International Airport", "ICN"),
        "rail-2": ("Hong Kong International Airport", "HKG"),
        "rail-4": ("Indira Gandhi International Airport", "DEL"),
    },
    "north-america-1": {
        "rail-1": ("John F. Kennedy International Airport", "JFK"),
        "rail-2": ("O'Hare International Airport", "ORD"),
        "rail-3": ("Hartsfield–Jackson Atlanta International Airport", "ATL"),
        "rail-4": ("Los Angeles International Airport", "LAX"),
    },
    "south-america-1": {
        "rail-1": ("São Paulo–Guarulhos International Airport", "GRU"),
        "rail-2": ("Ministro Pistarini International Airport", "EZE"),
        "rail-3": ("El Dorado International Airport", "BOG"),
        "rail-4": ("Arturo Merino Benítez Airport", "SCL"),
    },
    "middle-east-1": {
        "rail-1": ("Dubai International Airport", "DXB"),
        "rail-2": ("Hamad International Airport", "DOH"),
        "rail-3": ("Istanbul Airport", "IST"),
        "rail-4": ("Abu Dhabi International Airport", "AUH"),
    },
    "oceania-1": {
        "rail-1": ("Sydney Kingsford Smith Airport", "SYD"),
        "rail-3": ("Auckland Airport", "AKL"),
    },
    "central-america-1": {
        "rail-1": ("Tocumen International Airport", "PTY"),
        "rail-3": ("Cancún International Airport", "CUN"),
    },
}


def letters_only(s: str) -> str:
    s = unicodedata.normalize("NFKD", s)
    s = "".join(c for c in s if not unicodedata.combining(c))
    return re.sub(r"[^A-Za-z]", "", s).upper()


def next_code(base: str, used: set[str]) -> str:
    if base not in used:
        return base
    letters = letters_only(base) or "X"
    stem = letters[:2] if len(letters) >= 2 else letters
    n = 2
    while f"{stem}{n}" in used:
        n += 1
    return f"{stem}{n}"


def main() -> None:
    data = json.loads(PATH.read_text())

    for loc in data:
        st = loc.get("specialType")
        if st == "jail":
            loc["assets"] = {"icon": PRISON}
        elif st == "tax":
            loc["assets"] = {"icon": TAX}
        elif st == "go_to_jail":
            loc["assets"] = {"icon": PRISON}

        world = loc["worldId"]
        if loc["kind"] == "railroad" and world in AIRPORTS and loc["slug"] in AIRPORTS[world]:
            name, code = AIRPORTS[world][loc["slug"]]
            loc["name"] = name
            loc["description"] = name
            loc["aboutShort"] = f"{name} ({code})"
            loc["boardCode"] = code
            loc["assets"] = {"icon": PLANE}

    # Resolve boardCode collisions within each world (airports win; bump others)
    by_world: dict[str, list[dict]] = defaultdict(list)
    for loc in data:
        by_world[loc["worldId"]].append(loc)

    for world_id, locs in by_world.items():
        reserved: set[str] = set()
        for loc in locs:
            if loc["kind"] == "railroad" and world_id in AIRPORTS and loc["slug"] in AIRPORTS[world_id]:
                reserved.add(loc["boardCode"])

        used = set(reserved)
        for loc in locs:
            if loc["kind"] == "railroad" and world_id in AIRPORTS and loc["slug"] in AIRPORTS[world_id]:
                continue
            code = loc.get("boardCode") or letters_only(loc["name"])[:3] or "XXX"
            if code in used:
                code = next_code(code, used)
                loc["boardCode"] = code
            used.add(code)

        codes = [l["boardCode"] for l in locs]
        dups = [k for k, v in Counter(codes).items() if v > 1]
        if dups:
            raise SystemExit(f"dup in {world_id}: {dups}")

    PATH.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
    print(f"updated {PATH}")
    for world_id in sorted(AIRPORTS):
        print(world_id)
        for loc in sorted(by_world[world_id], key=lambda x: x["boardIndex"]):
            if loc["kind"] == "railroad":
                print(f"  {loc['slug']:8} {loc['boardCode']:4} {loc['name']}")


if __name__ == "__main__":
    main()
