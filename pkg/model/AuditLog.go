package model

import (
	"time"
)

type AuditLog struct {
	CreatedOn time.Time `sql:"created_on,type:timestamptz"`
	CreatedBy int32     `sql:"created_by,type:integer"`
	UpdatedOn time.Time `sql:"updated_on,type:timestamptz"`
	UpdatedBy int32     `sql:"updated_by,type:integer"`
}
