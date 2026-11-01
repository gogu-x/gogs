"""Regression checks for loadout TSV validation; never imports configuration into MongoDB."""

import csv
import importlib.util
import io
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("loadout_validator", Path(__file__).with_name("validate-loadout-config.py"))
VALIDATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VALIDATOR)
SOURCE = Path(__file__).resolve().parents[2] / "glconf"


class LoadoutConfigTests(unittest.TestCase):
    def setUp(self):
        self.sources = {name + ".tsv": (SOURCE / (name + ".tsv")).read_text(encoding="utf-8-sig")
                        for name in (*VALIDATOR.TABLES, "BattleRoleCfg")}

    def validate(self):
        def open_fixture(path, *args, **kwargs):
            return io.StringIO(self.sources[path.name], newline="")
        with patch.object(Path, "open", open_fixture):
            return VALIDATOR.validate(SOURCE)

    def change(self, table, record_id, column, value):
        name = table + ".tsv"
        rows = list(csv.reader(io.StringIO(self.sources[name]), delimiter="\t"))
        index = rows[4].index(column)
        for row in rows[9:]:
            if row and row[0] == str(record_id):
                row[index] = value
                break
        else:
            self.fail("fixture record missing")
        target = io.StringIO(newline="")
        csv.writer(target, delimiter="\t").writerows(rows)
        self.sources[name] = target.getvalue()

    def test_current_tables(self):
        counts = self.validate()
        self.assertEqual(set(counts), set(VALIDATOR.TABLES))
        self.assertTrue(all(count > 0 for count in counts.values()))

    def test_missing_equipment_reference(self):
        self.change("PlayerInitRoleCfg", 1, "equipment_config_ids", "[99999]")
        with self.assertRaisesRegex(VALIDATOR.ConfigError, "inventory references"):
            self.validate()

    def test_duplicate_jade_in_two_slots(self):
        self.change("LoadoutInitCfg", 1, "equipped_soul_jade_config_ids", "[1,1,3,4,5,6]")
        with self.assertRaisesRegex(VALIDATOR.ConfigError, "repeated or missing equipped jade"):
            self.validate()

    def test_talent_cycle(self):
        self.change("TalentNodeCfg", 1, "parent_id", "4")
        with self.assertRaisesRegex(VALIDATOR.ConfigError, "cyclic prerequisite"):
            self.validate()

    def test_negative_material_cost(self):
        self.change("SoulJadeLevelCfg", 1, "upgrade_cost", "-1")
        with self.assertRaisesRegex(VALIDATOR.ConfigError, "invalid material cost"):
            self.validate()

    def test_array_boolean_is_not_an_integer(self):
        self.change("SoulJadeLevelCfg", 1, "allowed_attributes", "[true,2,3,4]")
        with self.assertRaisesRegex(VALIDATOR.ConfigError, "expected int32"):
            self.validate()

    def test_int32_overflow(self):
        self.change("TalentNodeCfg", 1, "attack_permille", "2147483648")
        with self.assertRaisesRegex(VALIDATOR.ConfigError, "int32 overflow"):
            self.validate()


if __name__ == "__main__":
    unittest.main()
