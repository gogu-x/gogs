package role

// Role 保存角色模块拥有的养成、技能和装备穿戴关系。
// +deepcopy-gen=true
type Role struct {
	ID           string   `bson:"id"`
	ConfigID     int32    `bson:"config_id"`
	Level        int32    `bson:"level"`
	Star         int32    `bson:"star"`
	Breakthrough int32    `bson:"breakthrough"`
	Skills       []Skill  `bson:"skills"`
	EquipmentIDs []string `bson:"equipment_ids"`
}

// Skill 保存角色已配置技能及其等级。
// +deepcopy-gen=true
type Skill struct {
	ConfigID int32 `bson:"config_id"`
	Level    int32 `bson:"level"`
}

// Mgr 管理玩家拥有的角色数据。
// +deepcopy-gen=true
type Mgr struct {
	Character *Role `bson:"character"`
}

// New 创建空的角色模块。
func New() *Mgr { return &Mgr{} }

// GetCharacter 返回当前主角色。
func (m *Mgr) GetCharacter() (*Role, bool) {
	if m == nil || m.Character == nil {
		return nil, false
	}
	return m.Character, true
}

// GetID 返回角色实例 ID。
func (r *Role) GetID() string {
	if r == nil {
		return ""
	}
	return r.ID
}

// GetConfigID 返回角色配置 ID。
func (r *Role) GetConfigID() int32 {
	if r == nil {
		return 0
	}
	return r.ConfigID
}

// GetLevel 返回角色等级。
func (r *Role) GetLevel() int32 {
	if r == nil {
		return 0
	}
	return r.Level
}

// GetEquipmentIDs 返回角色当前穿戴的装备实例 ID 列表副本。
func (r *Role) GetEquipmentIDs() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.EquipmentIDs...)
}

// SetEquipmentIDs 替换角色当前穿戴的装备实例 ID 列表。
func (r *Role) SetEquipmentIDs(ids []string) {
	if r == nil {
		return
	}
	r.EquipmentIDs = append([]string(nil), ids...)
}

// BattleCharacter 返回用于战斗组装的角色副本。
func (m *Mgr) BattleCharacter() (*Role, bool) {
	if m == nil || m.Character == nil || m.Character.ID == "" {
		return nil, false
	}
	copyRole := *m.Character
	copyRole.Skills = append([]Skill(nil), m.Character.Skills...)
	copyRole.EquipmentIDs = append([]string(nil), m.Character.EquipmentIDs...)
	return &copyRole, true
}
