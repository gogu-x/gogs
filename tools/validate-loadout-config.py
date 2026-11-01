#!/usr/bin/env python3
"""Validate loadout TSV metadata, value types and references without database access."""

from __future__ import annotations

import argparse
import csv
import json
import re
from pathlib import Path

from config_source import ConfigError, config_records, read_config

TABLES = (
    "BattleEquipCfg", "PlayerInitRoleCfg", "EquipmentSlotCfg", "LoadoutInitCfg",
    "SoulJadeCfg", "SoulJadeLevelCfg", "TalentBranchCfg", "TalentNodeCfg", "TalentRuleCfg",
)
INTEGER = re.compile(r"^[+-]?[0-9]+$")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ConfigError(message)


def integer(value: object, bits: int, where: str) -> None:
    require(type(value) is int, f"{where}: expected int{bits}, got {value!r}")
    require(-(2 ** (bits - 1)) <= value < 2 ** (bits - 1), f"{where}: int{bits} overflow")


def object_pairs(pairs: list[tuple[str, object]]) -> dict:
    result = {}
    for key, value in pairs:
        require(key not in result, f"duplicate JSON field {key!r}")
        result[key] = value
    return result


def value_type(raw: str, declared: str, where: str) -> None:
    if declared in ("int", "long"):
        require(bool(INTEGER.fullmatch(raw)), f"{where}: invalid integer syntax {raw!r}")
        integer(int(raw), 32 if declared == "int" else 64, where)
    elif declared == "bool":
        require(raw.lower() in ("0", "1", "true", "false"), f"{where}: invalid boolean")
    elif declared == "string":
        return
    elif declared in ("int[]", "TypIDVal[]"):
        try:
            values = json.loads(raw, object_pairs_hook=object_pairs)
        except (json.JSONDecodeError, ConfigError) as error:
            raise ConfigError(f"{where}: {error}") from error
        require(isinstance(values, list), f"{where}: expected JSON array")
        for index, value in enumerate(values):
            child = f"{where}[{index}]"
            if declared == "int[]":
                integer(value, 32, child)
                continue
            require(isinstance(value, dict), f"{child}: expected TypIDVal object")
            require(set(value) == {"typ", "id", "val", "pro"}, f"{child}: invalid TypIDVal fields")
            require(value["typ"] == "battle_attribute", f"{child}.typ: unsupported equipment modifier")
            integer(value["id"], 32, child + ".id")
            integer(value["val"], 64, child + ".val")
            integer(value["pro"], 32, child + ".pro")
            require(value["id"] in (1, 2, 3, 4), f"{child}.id: unsupported battle attribute")
    else:
        raise ConfigError(f"{where}: unsupported declared type {declared!r}")


def load_table(root: Path, name: str) -> dict[int, dict]:
    path = root / (name + ".tsv")
    physical = []
    with path.open(encoding="utf-8-sig", newline="") as source:
        reader = csv.reader(source, delimiter="\t", strict=True)
        for row in reader:
            physical.append((reader.line_num, row))
    require(len(physical) >= 10, f"{path}: missing metadata or data")
    headers = [row for _, row in physical[:7]]
    width = len(headers[0])
    for index, row in enumerate(headers):
        require(len(row) == width, f"{path}:{index+1}: metadata column count differs")
    for index in (3, 4):
        require(len(set(headers[index])) == width and all(headers[index]),
                f"{path}:{index+1}: empty or duplicate field name")
    for column, declared in enumerate(headers[2]):
        require(headers[1][column] == "s", f"{path}: unexpected field domain")
        prefix = "ARR" if declared.endswith("[]") else {"int": "INT", "long": "LONG", "string": "STR", "bool": "BOL"}.get(declared)
        require(prefix is not None and headers[0][column].split("_", 2)[1] == prefix,
                f"{path}: type tag mismatch at column {column+1}")
        if declared == "TypIDVal[]":
            require(headers[5][column] == "TypIDVal_P_cspb", f"{path}: wrong modifier structure binding")
    for line, row in physical[9:]:
        if not any(row):
            continue
        require(len(row) == width, f"{path}:{line}: expected {width} columns, got {len(row)}")
        for index, raw in enumerate(row):
            value_type(raw, headers[2][index], f"{path}:{line}:{headers[4][index]}")
    parsed = config_records(read_config(path))
    result = {}
    for record in parsed:
        record_id = record["id"]
        require(record_id not in result, f"{path}: duplicate id {record_id}")
        result[record_id] = record
    require(bool(result), f"{path}: empty table")
    return result


def validate(root: Path) -> dict[str, int]:
    tables = {name: load_table(root, name) for name in TABLES}
    slots, equipment = tables["EquipmentSlotCfg"], tables["BattleEquipCfg"]
    role_ids = {row["id"] for row in config_records(read_config(root / "BattleRoleCfg.tsv"))}
    for item_id, row in equipment.items():
        require(row["slot"] in slots, f"BattleEquipCfg id={item_id}: missing slot")
        require(row["initial_level"] > 0 and row["initial_refine_level"] >= 0,
                f"BattleEquipCfg id={item_id}: invalid initial progression")
        require(all(row[key] for key in ("name", "rarity", "icon_key")), f"BattleEquipCfg id={item_id}: missing display metadata")
    for row in tables["PlayerInitRoleCfg"].values():
        owned, equipped = row["equipment_config_ids"], row["equipped_config_ids"]
        require(row["role_config_id"] in role_ids, "PlayerInitRoleCfg: missing battle role")
        require(len(owned) == len(set(owned)) and set(owned) <= equipment.keys(), "PlayerInitRoleCfg: invalid inventory references")
        require(len(equipped) == len(set(equipped)) and set(equipped) <= set(owned), "PlayerInitRoleCfg: invalid equipped references")
        equipped_slots = [equipment[item]["slot"] for item in equipped]
        require(len(equipped_slots) == len(set(equipped_slots)), "PlayerInitRoleCfg: duplicate equipped slot")
    levels, jades = tables["SoulJadeLevelCfg"], tables["SoulJadeCfg"]
    previous = None
    for level, row in sorted(levels.items()):
        where = f"SoulJadeLevelCfg id={level}"
        attributes = row["allowed_attributes"]
        require(level > 0, where + ": invalid level")
        require(len(attributes) == len(set(attributes)) and set(attributes) <= {1, 2, 3, 4}, where + ": invalid attribute pool")
        require(1 <= row["affix_count"] <= len(attributes), where + ": affix count exceeds pool")
        require(row["refine_cost"] > 0 and row["upgrade_cost"] >= 0, where + ": invalid material cost")
        require(level == max(levels) or row["upgrade_cost"] > 0, where + ": non-final upgrade is free")
        require(0 <= row["refine_min_permille"] <= row["refine_max_permille"], where + ": invalid refine range")
        require(row["new_affix_permille"] >= 0 and row["upgrade_permille"] >= 0, where + ": negative growth value")
        if previous is not None:
            require(row["affix_count"] >= previous["affix_count"], where + ": upgrading removes affixes")
            require(set(previous["allowed_attributes"]) <= set(attributes), where + ": upgrading invalidates existing affixes")
        previous = row
    for item_id, row in jades.items():
        require(row["initial_attribute"] in (1, 2, 3, 4) and row["initial_permille"] >= 0,
                f"SoulJadeCfg id={item_id}: invalid initial affix")
        require(all(row[key] for key in ("name", "rarity", "icon_key")), f"SoulJadeCfg id={item_id}: missing display metadata")
    for row in tables["LoadoutInitCfg"].values():
        owned, equipped = row["soul_jade_config_ids"], row["equipped_soul_jade_config_ids"]
        require(len(owned) == len(set(owned)) and set(owned) <= jades.keys(), "LoadoutInitCfg: invalid jade inventory")
        nonempty = [item for item in equipped if item != 0]
        require(len(nonempty) == len(set(nonempty)) and set(nonempty) <= set(owned), "LoadoutInitCfg: repeated or missing equipped jade")
        require(0 < row["soul_jade_slot_count"] == len(equipped), "LoadoutInitCfg: slot count mismatch")
        require(row["soul_jade_level"] in levels and row["soul_jade_essence"] >= 0, "LoadoutInitCfg: invalid initial progression")
        pool = levels[row["soul_jade_level"]]["allowed_attributes"]
        require(all(jades[item]["initial_attribute"] in pool for item in owned), "LoadoutInitCfg: initial affix outside level pool")
    nodes, branches = tables["TalentNodeCfg"], tables["TalentBranchCfg"]
    positions, roots = set(), {}
    for node_id, row in nodes.items():
        where = f"TalentNodeCfg id={node_id}"
        branch, parent = row["branch_id"], row["parent_id"]
        require(branch in branches, where + ": missing branch")
        position = (branch, row["ui_index"])
        require(row["ui_index"] >= 0 and position not in positions, where + ": invalid or duplicate UI position")
        positions.add(position)
        require(all(row[key] for key in ("name", "effect", "icon_key")), where + ": missing display metadata")
        require(all(row[key] >= 0 for key in ("max_hp_permille", "attack_permille", "defense_permille", "speed_permille")), where + ": negative growth value")
        if row["is_root"]:
            require(parent == -1 and row["point_cost"] == 0, where + ": root must be free and parentless")
            roots[branch] = roots.get(branch, 0) + 1
        else:
            require(row["point_cost"] > 0 and parent in nodes, where + ": missing prerequisite or invalid cost")
            require(nodes[parent]["branch_id"] == branch, where + ": prerequisite crosses branch")
        visited, current = set(), node_id
        while current != -1:
            require(current in nodes and current not in visited, where + ": missing or cyclic prerequisite")
            visited.add(current)
            current = nodes[current]["parent_id"]
    require(all(roots.get(branch) == 1 for branch in branches), "TalentBranchCfg: each branch needs one free root")
    total_cost = sum(row["point_cost"] for row in nodes.values())
    for row in tables["TalentRuleCfg"].values():
        require(all(row[key] >= 0 for key in ("base_points", "level_point_start", "points_per_level", "points_per_breakthrough")), "TalentRuleCfg: negative point rule")
        require(row["base_points"] <= row["max_points"] <= total_cost, "TalentRuleCfg: point cap does not fit configured nodes")
    return {name: len(records) for name, records in tables.items()}


def self_test() -> None:
    value_type("2147483647", "int", "test")
    value_type("[1,2,3]", "int[]", "test")
    for value, declared in (("2147483648", "int"), ("1.0", "int"), ("[true]", "int[]"), ("{}", "int[]"), ("nan", "long")):
        try:
            value_type(value, declared, "negative fixture")
        except ConfigError:
            continue
        raise AssertionError(f"invalid value accepted: {value!r}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("config_path", type=Path)
    args = parser.parse_args()
    self_test()
    try:
        counts = validate(args.config_path.resolve())
    except (ConfigError, OSError, csv.Error) as error:
        raise SystemExit(str(error))
    print("Loadout TSV validation passed (working tree; no database access):")
    for name, count in counts.items():
        print(f"  {name}: {count} records")
