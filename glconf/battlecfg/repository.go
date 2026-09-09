package battlecfg

import (
	"fmt"
	"sync"

	"github.com/gogu-x/gogs/battle/iproto"
)

// ConfigRepository 提供按版本读取、不可变写入的多版本配置仓库。
// 同一 version 一旦写入即内容不可变（由稳定指纹保证），Get 返回深拷贝。
type ConfigRepository interface {
	Put(iproto.Config) error
	Get(version string) (iproto.Config, error)
}

type MemoryConfigRepository struct {
	mu       sync.RWMutex
	versions map[string]iproto.Config
	hashes   map[string]string
}

func NewMemoryConfigRepository() *MemoryConfigRepository {
	return &MemoryConfigRepository{versions: map[string]iproto.Config{}, hashes: map[string]string{}}
}

func (r *MemoryConfigRepository) Put(c iproto.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	hash, err := StableConfigHash(c)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, exists := r.hashes[c.Version]; exists && old != hash {
		return fmt.Errorf("config version %q already exists with different content", c.Version)
	}
	r.versions[c.Version] = cloneConfig(c)
	r.hashes[c.Version] = hash
	return nil
}

func (r *MemoryConfigRepository) Get(version string) (iproto.Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.versions[version]
	if !ok {
		return iproto.Config{}, fmt.Errorf("config version %q not found", version)
	}
	return cloneConfig(c), nil
}
