#!/usr/bin/env python3
"""Import planner TSV/JSON records into MongoDB collections.

Usage: import-conf.py <mongo_uri> <mongo_db> <config_path>
"""

from __future__ import annotations

import argparse
import csv
import json
import sys
import uuid
from pathlib import Path
from typing import Any

from config_source import (
    ConfigError,
    config_records,
    ensure_identifier,
    read_config,
    source_files,
)

MONGO_TIMEOUT_MS = 10_000


def load_datasets(config_path: Path) -> dict[str, list[dict[str, Any]]]:
    if not config_path.is_dir():
        raise ConfigError(f"config path is not a directory: {config_path}")
    datasets: dict[str, list[dict[str, Any]]] = {}
    for path in source_files(config_path):
        fields = read_config(path)
        collection_name = ensure_identifier(fields["tablename"], "collection")
        if collection_name in datasets:
            raise ConfigError(f"duplicate configuration collection: {collection_name}")
        datasets[collection_name] = config_records(fields)
    return datasets


def import_mongo(
    datasets: dict[str, list[dict[str, Any]]],
    mongo_uri: str,
    database_name: str,
) -> None:
    try:
        from pymongo import ASCENDING, MongoClient
    except ImportError as error:
        raise ConfigError(
            "pymongo is required; install tools/requirements.txt"
        ) from error

    client = MongoClient(
        mongo_uri,
        serverSelectionTimeoutMS=MONGO_TIMEOUT_MS,
        connectTimeoutMS=MONGO_TIMEOUT_MS,
    )
    try:
        client.admin.command("ping")
        database = client[database_name]
        for collection_name, records in sorted(datasets.items()):
            temporary_name = f"__conf_import_{collection_name}_{uuid.uuid4().hex}"
            temporary = database[temporary_name]
            try:
                if records:
                    temporary.insert_many(records, ordered=True)
                temporary.create_index(
                    [("__index__", ASCENDING)],
                    unique=True,
                    name="__index___1",
                )
                temporary.rename(collection_name, dropTarget=True)
            except Exception:
                database.drop_collection(temporary_name)
                raise
            print(
                f"imported {len(records)} records into "
                f"{database_name}.{collection_name}"
            )
    except Exception as error:
        raise ConfigError(f"MongoDB import failed: {error}") from error
    finally:
        client.close()


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Import planner configuration records into MongoDB."
    )
    parser.add_argument("mongo_uri")
    parser.add_argument("mongo_db")
    parser.add_argument("config_path", type=Path)
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv[1:])
    try:
        datasets = load_datasets(args.config_path.resolve())
        record_count = sum(len(records) for records in datasets.values())
        print(f"validated {record_count} records in {len(datasets)} collections")
        import_mongo(datasets, args.mongo_uri, args.mongo_db)
    except (ConfigError, OSError, csv.Error, json.JSONDecodeError) as error:
        print(f"import-conf: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
