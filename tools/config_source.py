"""Read and validate planner TSV/JSON configuration sources."""

from __future__ import annotations

import csv
import json
import re
from pathlib import Path
from typing import Any

DATA_TYPES = {
    "int": "int32",
    "long": "int64",
    "float": "float32",
    "double": "float64",
    "string": "string",
    "bool": "bool",
}
TSV_DATA_START = 9
GO_IDENTIFIER = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


class ConfigError(ValueError):
    """Reports an invalid configuration source with its file context."""


def camel_to_snake(text: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "_", text).lower()


def ensure_identifier(value: str, label: str) -> str:
    value = value.strip()
    if not GO_IDENTIFIER.fullmatch(value):
        raise ConfigError(f"invalid Go identifier for {label}: {value!r}")
    return value


def normalize_rows(rows: list[list[str]]) -> list[list[str]]:
    width = max((len(row) for row in rows), default=0)
    return [row + [""] * (width - len(row)) for row in rows]


def read_tsv(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8-sig", newline="") as source:
        rows = normalize_rows(list(csv.reader(source, delimiter="\t")))
    if len(rows) < 6:
        raise ConfigError(
            f"{path}: expected at least 6 metadata rows, got {len(rows)}"
        )

    fields = {
        "tablename": path.stem,
        "domain": rows[1],
        "type": rows[2],
        "uppername": rows[3],
        "lowername": rows[4],
        "param": rows[5],
        "comment": rows[6] if len(rows) > 6 else [],
        "rows": rows[TSV_DATA_START:],
        "source": path,
    }
    for index, name in enumerate(fields["lowername"]):
        if not name and fields["uppername"][index]:
            fields["lowername"][index] = camel_to_snake(fields["uppername"][index])
    return fields


def infer_json(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8-sig") as source:
        value = json.load(source)
    records = value if isinstance(value, list) else [value]
    if not records or not isinstance(records[0], dict):
        raise ConfigError(f"{path}: JSON must contain an object or object array")

    names = list(records[0])
    type_names: list[str] = []
    for name in names:
        item = records[0][name]
        if isinstance(item, bool):
            type_names.append("bool")
        elif isinstance(item, int):
            type_names.append("long" if abs(item) > 2_147_483_647 else "int")
        elif isinstance(item, float):
            type_names.append("double")
        elif isinstance(item, str):
            type_names.append("string")
        else:
            raise ConfigError(
                f"{path}: field {name!r} needs an explicit TSV type"
            )

    upper_names = [
        "ID" if name == "id" else "".join(part.capitalize() for part in name.split("_"))
        for name in names
    ]
    return {
        "tablename": path.stem,
        "domain": ["s"] * len(names),
        "type": type_names,
        "uppername": upper_names,
        "lowername": names,
        "param": [""] * len(names),
        "comment": [""] * len(names),
        "records": records,
        "source": path,
    }


def read_config(path: Path) -> dict[str, Any]:
    return read_tsv(path) if path.suffix.lower() == ".tsv" else infer_json(path)


def parse_cell(
    raw_value: str,
    raw_type: str,
    path: Path,
    row_number: int,
    field_name: str,
) -> Any:
    value = raw_value.strip()
    type_name = raw_type.strip()
    is_array = type_name.endswith("[]")
    base_type = type_name[:-2] if is_array else type_name
    try:
        if is_array or base_type not in DATA_TYPES:
            if value == "" and is_array:
                return []
            return json.loads(value)
        if base_type in {"int", "long"}:
            return int(value)
        if base_type in {"float", "double"}:
            return float(value)
        if base_type == "bool":
            normalized = value.lower()
            if normalized in {"1", "true"}:
                return True
            if normalized in {"0", "false", ""}:
                return False
            raise ValueError("expected true/false or 1/0")
        return raw_value
    except (TypeError, ValueError, json.JSONDecodeError) as error:
        raise ConfigError(
            f"{path}:{row_number}: invalid {type_name} value for {field_name}: "
            f"{raw_value!r} ({error})"
        ) from error


def tsv_records(fields: dict[str, Any]) -> list[dict[str, Any]]:
    path = fields["source"]
    records: list[dict[str, Any]] = []
    for offset, row in enumerate(fields["rows"]):
        if not any(cell.strip() for cell in row):
            continue
        record: dict[str, Any] = {}
        for index, domain in enumerate(fields["domain"]):
            if "s" not in domain.lower():
                continue
            field_name = fields["lowername"][index].strip()
            if not field_name:
                raise ConfigError(
                    f"{path}:{offset + TSV_DATA_START + 1}: missing BSON field name "
                    f"for column {index + 1}"
                )
            record[field_name] = parse_cell(
                row[index],
                fields["type"][index],
                path,
                offset + TSV_DATA_START + 1,
                field_name,
            )
        record["__index__"] = len(records)
        records.append(record)
    return records


def config_records(fields: dict[str, Any]) -> list[dict[str, Any]]:
    if "rows" in fields:
        return tsv_records(fields)
    records: list[dict[str, Any]] = []
    for index, value in enumerate(fields["records"]):
        if not isinstance(value, dict):
            raise ConfigError(f"{fields['source']}: record {index + 1} is not an object")
        record = dict(value)
        record["__index__"] = index
        records.append(record)
    return records


def source_files(config_path: Path) -> list[Path]:
    files = [
        path
        for path in config_path.rglob("*")
        if path.is_file()
        and not path.name.startswith((".", "~"))
        and path.suffix.lower() in {".tsv", ".json"}
    ]
    if not files:
        raise ConfigError(f"no .tsv or .json files found in {config_path}")
    return sorted(files)
