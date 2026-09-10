"""Go templates used by gen-conf.py."""


def get_table_template(key_type="int32"):
    getter = "Get0(key)" if key_type == "string" else "Get(id)"
    key_arg = "key string" if key_type == "string" else "id int32"
    no_err = "" if key_type == "string" else '''
// Get{{struct_name}}NoErr 按 ID 查询配置；不存在时返回 nil。
func Get{{struct_name}}NoErr(id int32) *{{struct_name}} {
    if table := asCsv(csv{{struct_name}}); table != nil {
        if cfg := table.GetNoErr(id); cfg != nil {
            return cfg.(*{{struct_name}})
        }
    }
    return nil
}
'''
    interface_getter = "" if key_type == "string" else '''
// Get{{struct_name}}Interface 按 ID 查询配置并返回通用接口。
func Get{{struct_name}}Interface(id int32) interface{} {
    if table := asCsv(csv{{struct_name}}); table != nil {
        return table.GetNoErr(id)
    }
    return nil
}
'''
    return '''// Package {{package_name}} 包含自动生成的游戏配置。
package {{package_name}}

import ({% for pkg in packages %}
    "{{pkg}}"{% endfor %}
)

// {{struct_name}}Key 是配置表的 MongoDB collection 名称。
const {{struct_name}}Key = "{{struct_name}}"

var csv{{struct_name}} atomic.Value

// {{struct_name}} 表示 {{struct_name}} 配置记录。
type {{struct_name}} struct {
{% for value in value_list %}    {{"%-20s\t%-18s"|format(value[0], value[1])}} `bson:"{{value[2]}}"` // {{value[3]}}
{% endfor %}}

func load{{struct_name}}(dbURI, dbName, confName string) (*CsvConf, error) {
    table, err := readCsv(dbURI, dbName, confName, &{{struct_name}}{})
    if err != nil {
        return nil, err
    }
    csv{{struct_name}}.Store(table)
    return table, nil
}

// Get{{struct_name}} 按配置键查询记录；不存在时返回 nil。
func Get{{struct_name}}({{key_arg}}) *{{struct_name}} {
    if table := asCsv(csv{{struct_name}}); table != nil {
        if cfg := table.{{getter}}; cfg != nil {
            return cfg.(*{{struct_name}})
        }
    }
    return nil
}
''' + no_err + '''
// GetAll{{struct_name}} 返回当前表的全部配置记录。
func GetAll{{struct_name}}() []*{{struct_name}} {
    table := asCsv(csv{{struct_name}})
    if table == nil {
        return nil
    }
    configs := make([]*{{struct_name}}, 0, len(table.Records))
    for _, record := range table.Records {
        configs = append(configs, record.(*{{struct_name}}))
    }
    return configs
}
''' + interface_getter


def get_registry_template():
    return '''// Package {{package_name}} 包含自动生成的游戏配置。
package {{package_name}}

import "fmt"

type confLoader func(dbURI, dbName, confName string) (*CsvConf, error)

type confMeta struct {
    source string
    loader confLoader
    csv    *CsvConf
}

var confs []*confMeta

// InitGameCfg 注册全部自动生成的配置表；可重复调用。
func InitGameCfg() {
    confs = confs[:0]
{% for name in names %}    confs = append(confs, &confMeta{source: {{name}}Key, loader: load{{name}}})
{% endfor %}}

// LoadAllConfs 从 MongoDB 加载全部配置表。
// dbURI 是 MongoDB URI，dbName 是配置数据库名。isHotLoad 保留用于兼容旧调用。
func LoadAllConfs(dbURI, dbName string, isHotLoad bool) error {
    _ = isHotLoad
    InitGameCfg()
    for _, meta := range confs {
        if err := loadConf(meta, dbURI, dbName); err != nil {
            return err
        }
    }
    return nil
}

// LoadConfs 从 MongoDB 加载指定配置表。
func LoadConfs(
    names []string,
    dbURI,
    dbName string,
    isHotLoad bool,
) error {
    _ = isHotLoad
    if len(confs) == 0 {
        InitGameCfg()
    }
    for _, name := range names {
        meta := getConfMeta(name)
        if meta == nil {
            return fmt.Errorf("configuration metadata not found: %s", name)
        }
        if err := loadConf(meta, dbURI, dbName); err != nil {
            return err
        }
    }
    return nil
}

func getConfMeta(name string) *confMeta {
    for _, meta := range confs {
        if meta.source == name {
            return meta
        }
    }
    return nil
}

func loadConf(meta *confMeta, dbURI, dbName string) error {
    table, err := meta.loader(dbURI, dbName, meta.source)
    if err != nil {
        return fmt.Errorf("load configuration %s: %w", meta.source, err)
    }
    meta.csv = table
    return nil
}
'''


def get_runtime_template():
    return '''// Package {{package_name}} 包含自动生成的游戏配置。
package {{package_name}}

import (
    "context"
    "errors"
    "fmt"
    "reflect"
    "sync"
    "sync/atomic"
    "time"

    "go.mongodb.org/mongo-driver/v2/bson"
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
    indexTag          = "index"
    compositeIndexTag = "index_composite"
    selectionTag      = "selection"
    mongoTimeout      = 10 * time.Second
)

// TypIDVal 表示配置表中的资产类型、ID 和数量。
type TypIDVal struct {
    Typ string `bson:"typ"` // 资产类型
    ID  int32  `bson:"id"`  // 资产 ID
    Val int64  `bson:"val"` // 资产数量
    Pro int32  `bson:"pro"` // 百分比参数
}

// CsvConf 保存一张配置表的记录和查询索引。
type CsvConf struct {
    Records              []interface{}                          // 全部配置记录
    Index                map[interface{}]interface{}            // 主键索引
    IndexComposite       map[interface{}]map[interface{}]interface{} // 联合索引
    SelectionOptionalKey map[interface{}]interface{}            // 字符串选择键索引
    typeRecord           reflect.Type
    source               string
}

var databaseCache sync.Map

func readCsv(
    dbURI,
    dbName,
    confName string,
    prototype interface{},
) (*CsvConf, error) {
    table, err := newCsv(prototype)
    if err != nil {
        return nil, err
    }
    database, err := mongoDatabase(dbURI, dbName)
    if err != nil {
        return nil, err
    }
    if err := table.read(database, confName); err != nil {
        return nil, err
    }
    return table, nil
}

func newCsv(prototype interface{}) (*CsvConf, error) {
    recordType := reflect.TypeOf(prototype)
    if recordType == nil {
        return nil, errors.New("configuration prototype is nil")
    }
    if recordType.Kind() == reflect.Ptr {
        recordType = recordType.Elem()
    }
    if recordType.Kind() != reflect.Struct {
        return nil, fmt.Errorf("configuration prototype must be a struct, got %s", recordType.Kind())
    }
    return &CsvConf{typeRecord: recordType}, nil
}

func mongoDatabase(uri, name string) (*mongo.Database, error) {
    if uri == "" || name == "" {
        return nil, errors.New("MongoDB URI and database name are required")
    }
    key := uri + "\\x00" + name
    if cached, ok := databaseCache.Load(key); ok {
        return cached.(*mongo.Database), nil
    }
    client, err := mongo.Connect(options.Client().ApplyURI(uri))
    if err != nil {
        return nil, fmt.Errorf("connect MongoDB: %w", err)
    }
    database := client.Database(name)
    actual, _ := databaseCache.LoadOrStore(key, database)
    return actual.(*mongo.Database), nil
}

func (table *CsvConf) read(database *mongo.Database, collection string) error {
    ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
    defer cancel()

    cursor, err := database.Collection(collection).Find(
        ctx,
        bson.D{},
        options.Find().SetSort(bson.D{ {Key: "__index__", Value: 1} }),
    )
    if err != nil {
        return err
    }
    defer cursor.Close(ctx)

    records := make([]interface{}, 0)
    for cursor.Next(ctx) {
        record := reflect.New(table.typeRecord)
        if err := cursor.Decode(record.Interface()); err != nil {
            return err
        }
        records = append(records, record.Interface())
    }
    if err := cursor.Err(); err != nil {
        return err
    }
    return table.replaceRecords(collection, records)
}

func (table *CsvConf) replaceRecords(source string, records []interface{}) error {
    index, err := table.buildIndex(records)
    if err != nil {
        return err
    }
    composite, err := table.buildCompositeIndex(records)
    if err != nil {
        return err
    }
    selection, err := table.buildSelectionIndex(records)
    if err != nil {
        return err
    }
    table.source = source
    table.Records = records
    table.Index = index
    table.IndexComposite = composite
    table.SelectionOptionalKey = selection
    return nil
}

func (table *CsvConf) buildIndex(records []interface{}) (map[interface{}]interface{}, error) {
    fieldIndex := taggedField(table.typeRecord, indexTag)
    if fieldIndex < 0 {
        fieldIndex = bsonField(table.typeRecord, "id")
    }
    if fieldIndex < 0 {
        return nil, fmt.Errorf("configuration %s has no primary index or bson id field", table.typeRecord)
    }
    result := make(map[interface{}]interface{}, len(records))
    for _, record := range records {
        key := reflect.ValueOf(record).Elem().Field(fieldIndex).Interface()
        if _, exists := result[key]; exists {
            return nil, fmt.Errorf("configuration %s has duplicate key %v", table.typeRecord, key)
        }
        result[key] = record
    }
    return result, nil
}

func (table *CsvConf) buildCompositeIndex(records []interface{}) (map[interface{}]map[interface{}]interface{}, error) {
    first := taggedFieldValue(table.typeRecord, compositeIndexTag, "1")
    second := taggedFieldValue(table.typeRecord, compositeIndexTag, "2")
    if first < 0 && second < 0 {
        return nil, nil
    }
    if first < 0 || second < 0 || first == second {
        return nil, fmt.Errorf("configuration %s has an invalid composite index", table.typeRecord)
    }
    result := make(map[interface{}]map[interface{}]interface{})
    for _, record := range records {
        value := reflect.ValueOf(record).Elem()
        firstKey := value.Field(first).Interface()
        secondKey := value.Field(second).Interface()
        if result[firstKey] == nil {
            result[firstKey] = make(map[interface{}]interface{})
        }
        if _, exists := result[firstKey][secondKey]; exists {
            return nil, fmt.Errorf("configuration %s has duplicate composite key %v/%v", table.typeRecord, firstKey, secondKey)
        }
        result[firstKey][secondKey] = record
    }
    return result, nil
}

func (table *CsvConf) buildSelectionIndex(records []interface{}) (map[interface{}]interface{}, error) {
    fieldIndex := taggedField(table.typeRecord, selectionTag)
    if fieldIndex < 0 {
        return nil, nil
    }
    result := make(map[interface{}]interface{}, len(records))
    for _, record := range records {
        key := reflect.ValueOf(record).Elem().Field(fieldIndex).Interface()
        if _, exists := result[key]; exists {
            return nil, fmt.Errorf("configuration %s has duplicate selection key %v", table.typeRecord, key)
        }
        result[key] = record
    }
    return result, nil
}

func taggedField(recordType reflect.Type, tag string) int {
    for index := 0; index < recordType.NumField(); index++ {
        if _, ok := recordType.Field(index).Tag.Lookup(tag); ok {
            return index
        }
    }
    return -1
}

func taggedFieldValue(recordType reflect.Type, tag, expected string) int {
    for index := 0; index < recordType.NumField(); index++ {
        if recordType.Field(index).Tag.Get(tag) == expected {
            return index
        }
    }
    return -1
}

func bsonField(recordType reflect.Type, expected string) int {
    for index := 0; index < recordType.NumField(); index++ {
        name := recordType.Field(index).Tag.Get("bson")
        if comma := len(name); comma > 0 {
            for offset, char := range name {
                if char == ',' {
                    name = name[:offset]
                    break
                }
            }
        }
        if name == expected {
            return index
        }
    }
    return -1
}

func asCsv(value atomic.Value) *CsvConf {
    loaded := value.Load()
    if loaded == nil {
        return nil
    }
    return loaded.(*CsvConf)
}

// Record 按位置返回一条配置记录。
func (table *CsvConf) Record(index int) interface{} {
    if table == nil || index < 0 || index >= len(table.Records) {
        return nil
    }
    return table.Records[index]
}

// NumRecord 返回配置记录数量。
func (table *CsvConf) NumRecord() int {
    if table == nil {
        return 0
    }
    return len(table.Records)
}

// Get 按 int32 主键查询配置记录。
func (table *CsvConf) Get(key int32) interface{} {
    if table == nil {
        return nil
    }
    return table.Index[key]
}

// GetNoErr 按 int32 主键查询配置记录。
func (table *CsvConf) GetNoErr(key int32) interface{} { return table.Get(key) }

// Get0 按字符串选择键查询配置记录。
func (table *CsvConf) Get0(key string) interface{} {
    if table == nil {
        return nil
    }
    return table.SelectionOptionalKey[key]
}

// GetExByInt32 按两个 int32 联合键查询配置记录。
func (table *CsvConf) GetExByInt32(first, second int32) interface{} {
    if table == nil || table.IndexComposite[first] == nil {
        return nil
    }
    return table.IndexComposite[first][second]
}

// GetExByInt32NoErr 按两个 int32 联合键查询配置记录。
func (table *CsvConf) GetExByInt32NoErr(first, second int32) interface{} {
    return table.GetExByInt32(first, second)
}

// SelectFun 是配置记录过滤函数。
type SelectFun func(record interface{}) bool

// Select 返回第一条满足过滤条件的记录。
func (table *CsvConf) Select(filter SelectFun) interface{} {
    if table == nil || filter == nil {
        return nil
    }
    for _, record := range table.Records {
        if filter(record) {
            return record
        }
    }
    return nil
}

// Filter 返回全部满足过滤条件的记录。
func (table *CsvConf) Filter(filter SelectFun) []interface{} {
    if table == nil || filter == nil {
        return nil
    }
    result := make([]interface{}, 0)
    for _, record := range table.Records {
        if filter(record) {
            result = append(result, record)
        }
    }
    return result
}
'''
