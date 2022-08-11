package types

import (
	"github.com/jitsucom/bulker/base/utils"
	"sort"
)

// Columns is a list of columns representation
type Columns map[string]SQLColumn

// TableField is a table column representation
type TableField struct {
	Field string      `json:"field,omitempty"`
	Type  string      `json:"type,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// Table is a dto for DWH Table representation
type Table struct {
	//Optional name of the schema. Not all databases
	Schema string
	Name   string

	Columns        Columns
	PKFields       utils.Set
	PrimaryKeyName string
	Partition      DatePartition

	DeletePkFields bool
}

// Exists returns true if there is at least one column
func (t *Table) Exists() bool {
	if t == nil {
		return false
	}

	return len(t.Columns) > 0 || len(t.PKFields) > 0 || t.DeletePkFields
}

// SortedColumnNames return column names sorted in alphabetical order
func (t *Table) SortedColumnNames() []string {
	columns := make([]string, 0, len(t.Columns))
	for name := range t.Columns {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	return columns
}

// Clone returns clone of current table
func (t *Table) Clone() *Table {
	clonedColumns := Columns{}
	for k, v := range t.Columns {
		clonedColumns[k] = v
	}

	clonedPkFields := t.PKFields.Clone()

	return &Table{
		Schema:         t.Schema,
		Name:           t.Name,
		Columns:        clonedColumns,
		PKFields:       clonedPkFields,
		PrimaryKeyName: t.PrimaryKeyName,
		DeletePkFields: t.DeletePkFields,
	}
}

// GetPKFields returns primary keys list
func (t *Table) GetPKFields() []string {
	return t.PKFields.ToSlice()
}

// GetPKFieldsSet returns primary keys set
func (t *Table) GetPKFieldsSet() utils.Set {
	return t.PKFields
}

// Diff calculates diff between current schema and another one.
// Return schema to add to current schema (for being equal) or empty if
// 1) another one is empty
// 2) all fields from another schema exist in current schema
// NOTE: Diff method doesn't take types into account
func (t *Table) Diff(another *Table) *Table {
	diff := &Table{Schema: t.Schema, Name: t.Name, Columns: map[string]SQLColumn{}, PKFields: utils.Set{}}

	if !another.Exists() {
		return diff
	}

	for name, column := range another.Columns {
		_, ok := t.Columns[name]
		if !ok {
			diff.Columns[name] = column
		}
	}

	jitsuPrimaryKeyName := BuildConstraintName(t.Schema, t.Name)
	//check if primary key is maintained by Jitsu (for Postgres and Redshift)
	if t.PrimaryKeyName != "" && t.PrimaryKeyName != jitsuPrimaryKeyName {
		//primary key isn't maintained by Jitsu: do nothing
		return diff
	}

	//primary keys logic
	if len(t.PKFields) > 0 {
		if !t.PKFields.Equals(another.PKFields) {
			//re-create or delete if another.PKFields is empty
			diff.DeletePkFields = true
			diff.PKFields = another.PKFields
			diff.PrimaryKeyName = jitsuPrimaryKeyName
		}
	} else if len(another.PKFields) > 0 {
		//create
		diff.PKFields = another.PKFields
		diff.PrimaryKeyName = jitsuPrimaryKeyName
	}

	return diff
}

func BuildConstraintName(schemaName string, tableName string) string {
	return schemaName + "_" + tableName + "_pk"
}
