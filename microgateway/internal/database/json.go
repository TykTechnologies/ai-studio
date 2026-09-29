package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/schema"
)

// JSON is a JSON column: JSONB on Postgres, JSON on SQLite. It is the JSON
// type of gorm.io/datatypes v1.2.6 (MIT licence, Copyright (c) 2013-NOW
// Jinzhu) without its MySQL handling, kept here so the gateway needs no
// module beyond gorm and its drivers. Its column types and the values it
// reads and writes must stay those of datatypes.JSON; the schema snapshot
// tests pin the column types.
type JSON json.RawMessage

// Value returns the JSON as a string, or NULL when it is empty.
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

// Scan reads a JSON column. Scanning NULL directly (database/sql Rows.Scan)
// gives the JSON value null, as datatypes.JSON v1.2.6 does. gorm does not call
// Scan for a NULL column when it loads a model: it leaves the field empty.
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	var bytes []byte
	if s, ok := value.(fmt.Stringer); ok {
		bytes = []byte(s.String())
	} else {
		switch v := value.(type) {
		case []byte:
			if len(v) > 0 {
				bytes = make([]byte, len(v))
				copy(bytes, v)
			}
		case string:
			bytes = []byte(v)
		default:
			return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
		}
	}

	result := json.RawMessage(bytes)
	*j = JSON(result)
	return nil
}

// MarshalJSON outputs the JSON itself rather than base64 of its bytes.
func (j JSON) MarshalJSON() ([]byte, error) {
	return json.RawMessage(j).MarshalJSON()
}

// UnmarshalJSON keeps the JSON as it is.
func (j *JSON) UnmarshalJSON(b []byte) error {
	result := json.RawMessage{}
	err := result.UnmarshalJSON(b)
	*j = JSON(result)
	return err
}

func (j JSON) String() string {
	return string(j)
}

// GormDataType is the dialect-independent type name.
func (JSON) GormDataType() string {
	return "json"
}

// GormDBDataType is the column type for each dialect.
func (JSON) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "JSON"
	case "postgres":
		return "JSONB"
	}
	return ""
}

// GormValue writes the JSON as a string parameter, or NULL when it is empty.
func (j JSON) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	if len(j) == 0 {
		return gorm.Expr("NULL")
	}
	data, _ := j.MarshalJSON()
	return gorm.Expr("?", string(data))
}
