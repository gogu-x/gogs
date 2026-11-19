package glconf

import (
	"errors"
	"slices"
	"testing"
)

func TestAfterLoadCallbacks(t *testing.T) {
	InitGameCfg()
	originalConfs := confs
	originalShopTable, _ := csvActvShopCfg.Load().(*CsvConf)
	originalCallbacks := afterLoadCallbacks
	originalRegister := registerConfigCallbacksOnce
	afterLoadCallbacks = make(map[string][]func(*CsvConf))
	registerConfigCallbacksOnce = func() {}
	t.Cleanup(func() {
		confs = originalConfs
		csvActvShopCfg.Store(originalShopTable)
		afterLoadCallbacks = originalCallbacks
		registerConfigCallbacksOnce = originalRegister
	})

	loadErr := errors.New("test load failed")
	shopID := int32(1)
	loadShop := func(_, database, _ string) (*CsvConf, error) {
		if database == "failed" {
			return nil, loadErr
		}
		table, err := newConfigForTest(&ActvShopCfg{}, []interface{}{&ActvShopCfg{CfgID: shopID}})
		if err == nil {
			csvActvShopCfg.Store(table)
		}
		return table, err
	}
	loadGoods := func(_, _, _ string) (*CsvConf, error) {
		return newCsv(&ActvShopGoodsCfg{})
	}
	confs = []confMeta{
		{source: ActvShopCfgKey, loader: loadShop},
		{source: ActvShopGoodsCfgKey, loader: loadGoods},
	}

	if RegisterAfterLoad("unknown", func(*CsvConf) {}) || RegisterAfterLoad(ActvShopCfgKey, nil) {
		t.Fatal("accepted an unknown key or nil callback")
	}
	var calls []string
	if !RegisterAfterLoad(ActvShopCfgKey, func(table *CsvConf) {
		if table != asCsv(csvActvShopCfg) {
			t.Fatal("callback did not receive the published shop table")
		}
		if cfg, _ := table.GetNoErr(shopID).(*ActvShopCfg); cfg.GetCfgID() != shopID {
			t.Fatalf("shop config %d unavailable in callback", shopID)
		}
		calls = append(calls, "shop-first")
	}) || !RegisterAfterLoad(ActvShopCfgKey, func(*CsvConf) {
		calls = append(calls, "shop-second")
	}) || !RegisterAfterLoad(ActvShopGoodsCfgKey, func(table *CsvConf) {
		if table == nil {
			t.Fatal("callback did not receive the empty goods table")
		}
		calls = append(calls, "goods")
	}) {
		t.Fatal("failed to register a callback")
	}

	if err := LoadAllConfs("", "", false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"shop-first", "shop-second", "goods"}; !slices.Equal(calls, want) {
		t.Fatalf("callbacks after full load = %v, want %v", calls, want)
	}

	calls = nil
	shopKeys := []string{ActvShopCfgKey}
	if err := LoadConfs(shopKeys, "", "failed", true); !errors.Is(err, loadErr) {
		t.Fatalf("load error = %v, want %v", err, loadErr)
	}
	if len(calls) != 0 {
		t.Fatalf("callbacks ran after failed load: %v", calls)
	}

	shopID++
	if err := LoadConfs(shopKeys, "", "", true); err != nil {
		t.Fatal(err)
	}
	if want := []string{"shop-first", "shop-second"}; !slices.Equal(calls, want) {
		t.Fatalf("callbacks after selected reload = %v, want %v", calls, want)
	}
}
