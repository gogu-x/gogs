package role

// Role 保存角色模块拥有的养成、技能和装备穿戴关系。
// +deepcopy-gen=true
type Role struct {
	ID           string   `bson:"id"`            // 角色实例 ID。
	ConfigID     int32    `bson:"config_id"`     // 角色配置 ID。
	Level        int32    `bson:"level"`         // 角色等级。
	Star         int32    `bson:"star"`          // 角色星级。
	Breakthrough int32    `bson:"breakthrough"`  // 角色突破等级。
	Skills       []Skill  `bson:"skills"`        // 角色技能列表。
	EquipmentIDs []string `bson:"equipment_ids"` // 当前穿戴的装备实例 ID。
}

// Skill 保存角色已配置技能及其等级。
// +deepcopy-gen=true
type Skill struct {
	ConfigID int32 `bson:"config_id"` // 技能配置 ID。
	Level    int32 `bson:"level"`     // 技能等级。
}

// Mgr 管理玩家拥有的角色数据。
// +deepcopy-gen=true
type Mgr struct {
	Character *Role `bson:"character"` // 当前主角色。
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

// SetCharacter 更新当前主角色。
func (m *Mgr) SetCharacter(character *Role) {
	if m != nil {
		m.Character = character
	}
}

// GetID 返回角色实例编号。
func (r *Role) GetID() string {
	if r == nil {
		return ""
	}
	return r.ID
}

// GetConfigID 返回角色配置编号。
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

// GetStar 返回角色星级。
func (r *Role) GetStar() int32 {
	if r == nil {
		return 0
	}
	return r.Star
}

// GetBreakthrough 返回角色突破等级。
func (r *Role) GetBreakthrough() int32 {
	if r == nil {
		return 0
	}
	return r.Breakthrough
}

// GetSkills 返回角色技能列表副本。
func (r *Role) GetSkills() []Skill {
	if r == nil {
		return nil
	}
	return append([]Skill(nil), r.Skills...)
}

// GetConfigID 返回技能配置编号。
func (s Skill) GetConfigID() int32 { return s.ConfigID }

// GetLevel 返回技能等级。
func (s Skill) GetLevel() int32 { return s.Level }

// GetEquipmentIDs 返回角色当前穿戴的装备实例编号列表副本。
func (r *Role) GetEquipmentIDs() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.EquipmentIDs...)
}

// AddEquipmentID 将装备实例编号加入角色穿戴列表。
func (r *Role) AddEquipmentID(id string) {
	if r == nil || id == "" {
		return
	}
	r.EquipmentIDs = append(r.EquipmentIDs, id)
}

// SetEquipmentIDs 替换角色当前穿戴的装备实例编号列表。
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
