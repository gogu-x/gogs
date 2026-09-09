package battlecfg

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/battle/iproto"
)

// LoadConfigRepository 从配置路径加载多版本引擎配置到内存仓库。
// path 为空时使用内置 default 开发配置（本地开发与测试自包含）。
func LoadConfigRepository(path string) (*MemoryConfigRepository, error) {
	repo := NewMemoryConfigRepository()
	if path == "" {
		if err := repo.Put(DefaultConfig()); err != nil {
			return nil, err
		}
		return repo, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var configs []iproto.Config
	if err := json.Unmarshal(data, &configs); err != nil {
		var single iproto.Config
		if singleErr := json.Unmarshal(data, &single); singleErr != nil {
			return nil, fmt.Errorf("battle config: expected object or array: %w", err)
		}
		configs = []iproto.Config{single}
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("battle config: no versions")
	}
	for _, config := range configs {
		if err := repo.Put(config); err != nil {
			return nil, err
		}
	}
	return repo, nil
}
