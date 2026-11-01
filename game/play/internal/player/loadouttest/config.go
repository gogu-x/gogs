// Package loadouttest loads source tables into isolated package test processes.
package loadouttest

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	"github.com/gogu-x/gogs/glconf"
)

// Load installs actual TSV records without a MongoDB connection or import.
func Load(t *testing.T) {
	t.Helper()
	loadTable(t, "SoulJadeCfg", glconf.SetSoulJadeCfgForTest)
	loadTable(t, "SoulJadeLevelCfg", glconf.SetSoulJadeLevelCfgForTest)
	loadTable(t, "LoadoutInitCfg", glconf.SetLoadoutInitCfgForTest)
	loadTable(t, "TalentBranchCfg", glconf.SetTalentBranchCfgForTest)
	loadTable(t, "TalentNodeCfg", glconf.SetTalentNodeCfgForTest)
	loadTable(t, "TalentRuleCfg", glconf.SetTalentRuleCfgForTest)
}

func loadTable[T any](t *testing.T, name string, install func([]*T) error) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve source directory")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(source), "../../../../../../glconf", name+".tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 10 {
		t.Fatalf("table has no records: %s", name)
	}
	fields := rows[3]
	var records []*T
	for _, cells := range rows[9:] {
		if len(cells) == 0 || cells[0] == "" {
			continue
		}
		record := new(T)
		value := reflect.ValueOf(record).Elem()
		for column, fieldName := range fields {
			if fieldName == "" || column >= len(cells) {
				continue
			}
			field := value.FieldByName(fieldName)
			if !field.IsValid() || !field.CanSet() {
				t.Fatalf("unknown field: %s.%s", name, fieldName)
			}
			cell := cells[column]
			if field.Kind() == reflect.String {
				field.SetString(cell)
				continue
			}
			if cell == "" {
				continue
			}
			if field.Kind() == reflect.Bool {
				parsed, err := strconv.ParseBool(cell)
				if err != nil {
					t.Fatal(err)
				}
				field.SetBool(parsed)
				continue
			}
			if err := json.Unmarshal([]byte(cell), field.Addr().Interface()); err != nil {
				t.Fatalf(
					"invalid %s.%s: %v", name, fieldName, err,
				)
			}
		}
		records = append(records, record)
	}
	if err := install(records); err != nil {
		t.Fatal(err)
	}
}
